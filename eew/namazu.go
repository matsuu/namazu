package eew

import (
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/antchfx/xmlquery"
	"golang.org/x/exp/slog"
)

const (
	DefaultZmqEndpoint = "tcp://127.0.0.1:5563"
)

type LatLng struct {
	Lat float64
	Lng float64
}

func (l *LatLng) String() string {
	if l == nil {
		return "経緯不明"
	}

	prefixLat := ""
	// 0はNとする
	if l.Lat >= 0 {
		prefixLat = "北緯"
	} else {
		prefixLat = "南緯"
	}
	lat := strconv.FormatFloat(math.Abs(l.Lat), 'f', -1, 64)

	// 0はEとする
	prefixLng := ""
	if l.Lng >= 0 {
		prefixLng = "東経"
	} else {
		prefixLng = "西経"
	}
	lng := strconv.FormatFloat(math.Abs(l.Lng), 'f', -1, 64)

	return fmt.Sprintf("%s%s度、%s%s度", prefixLat, lat, prefixLng, lng)
}

type Depth float64

// https://www.data.jma.go.jp/suishin/jyouhou/pdf/566.pdf
// > ・震源の深さがごく浅い場合（０ｋｍ）
// > （※「緊急地震速報（警報）」、「緊急地震速報（予報）」の場合、現行の運用では、震源の深さを「ごく浅い」とせずに、本要素の内容、属性「@description」において、震源の深さを 10km として扱い発表する。）
func (d *Depth) String() string {
	if d == nil {
		return "不明"
	}
	v := *d
	if v < 0 {
		v = -v
	}
	if v == 0 {
		v = 10000
	}
	return fmt.Sprintf("約%skm", strconv.FormatFloat(float64(v/1000), 'f', -1, 64))
}

type ReportTime time.Time

func (t *ReportTime) String() string {
	if t == nil {
		return "不明"
	}
	return time.Time(*t).Format("_2日15時04分")
}

type Intensity struct {
	From string
	To   string
}

func (i *Intensity) String() string {
	if i == nil {
		return "不明"
	}
	r := strings.NewReplacer("-", "弱", "+", "強")
	from := r.Replace(i.From)
	to := r.Replace(i.To)
	if to == "over" {
		return fmt.Sprintf("震度%s以上", from)
	}
	return fmt.Sprintf("震度%s", to)
}

type Magnitude string

func (m Magnitude) String() string {
	if m == "" {
		return "不明"
	}
	return fmt.Sprintf("%s", string(m))
}

type Serial int

func (s Serial) String() string {
	return fmt.Sprintf("第%d報", s)
}

type IsLast bool

func (l IsLast) String() string {
	if l {
		return " *最終報*"
	}
	return ""
}

type Content struct {
	EventId   string
	Time      *ReportTime
	AreaName  string
	LatLng    *LatLng
	Depth     *Depth
	Magnitude Magnitude
	Intensity *Intensity
	Serial    Serial
	IsLast    IsLast
	Url       string
}

func NewContent(r io.Reader) (*Content, error) {
	doc, err := xmlquery.Parse(r)
	if err != nil {
		slog.Error("Failed to parse xml data", err)
		return nil, err
	}

	content := Content{}

	root := xmlquery.FindOne(doc, "//Report")

	if n := root.SelectElement("//Head/EventID"); n != nil {
		content.EventId = n.InnerText()
	}

	if n := root.SelectElement("//Body/Earthquake/OriginTime"); n != nil {
		t, err := time.Parse("2006-01-02T15:04:05-07:00", n.InnerText())
		if err != nil {
			slog.Error("Failed to parse OriginTime", err)
			return nil, err
		}
		rt := ReportTime(t)
		content.Time = &rt
	} else if n := root.SelectElement("//Body/Earthquake/ArrivalTime"); n != nil {
		t, err := time.Parse("2006-01-02T15:04:05-07:00", n.InnerText())
		if err != nil {
			slog.Error("Failed to parse OriginTime", err)
			return nil, err
		}
		rt := ReportTime(t)
		content.Time = &rt
	}
	if n := root.SelectElement("//Body/Earthquake/Hypocenter/Area/Name"); n != nil {
		content.AreaName = n.InnerText()
	} else {
		content.AreaName = "不明"
	}
	if n := root.SelectElement("//Body/Earthquake/Hypocenter/Area/jmx_eb:Coordinate"); n != nil {
		coordinate := n.InnerText()
		if err := content.ParseCoordinate(coordinate); err != nil {
			slog.Error("Failed to parse coordinate", err)
		}
	}
	if n := root.SelectElement("//Body/Earthquake/jmx_eb:Magnitude"); n != nil {
		content.Magnitude = Magnitude(n.InnerText())
	}
	if n := root.SelectElement("//Body/Intensity/Forecast/ForecastInt"); n != nil {
		from := n.SelectElement("/From")
		to := n.SelectElement("/To")
		content.Intensity = &Intensity{
			From: from.InnerText(),
			To:   to.InnerText(),
		}
	}
	if n := root.SelectElement("//Head/Serial"); n != nil {
		serial, err := strconv.Atoi(n.InnerText())
		if err != nil {
			slog.Error("Failed to Atoi serial", err, slog.Any("serial", n.InnerText()))

		}
		content.Serial = Serial(serial)
	}
	if n := root.SelectElement("//Body/NextAdvisory"); n != nil {
		if n.InnerText() == "この情報をもって、緊急地震速報：最終報とします。" {
			content.IsLast = IsLast(true)
		}
	}

	if t, err := time.Parse("20060102150405", content.EventId); err == nil {
		content.Url = fmt.Sprintf("https://earthquake.tenki.jp/bousai/earthquake/detail/%s.html", t.Format("2006/01/02/2006-01-02-15-04-05"))
	}

	return &content, nil
}

func (c *Content) ParseCoordinate(coordinate string) error {
	var coordinates []string
	var s strings.Builder
	for i, r := range coordinate {
		switch r {
		case '/':
			coordinates = append(coordinates, s.String())
		case '+', '-':
			if i != 0 {
				coordinates = append(coordinates, s.String())
				s.Reset()
			}
			fallthrough
		default:
			s.WriteRune(r)
		}
	}
	// 全要素不明の場合がある
	if len(coordinates) > 0 {
		lat, err := strconv.ParseFloat(coordinates[0], 64)
		if err != nil {
			return err
		}
		lon, err := strconv.ParseFloat(coordinates[1], 64)
		if err != nil {
			return err
		}
		c.LatLng = &LatLng{lat, lon}
		// 深さだけがない場合がある
		if len(coordinates) > 2 {
			depth, err := strconv.ParseFloat(coordinates[2], 64)
			if err != nil {
				return err
			}
			d := Depth(depth)
			c.Depth = &d
		}
	}

	return nil
}

func (c Content) String() string {
	return fmt.Sprintf("**緊急地震速報（予報）** %s%s\n%sごろ、地震がありました。\n震源地は%s（%s）で震源の深さは%s、地震の規模（マグニチュード）は%s、この地震による最大震度は%sと推定されます。\n%s", c.Serial, c.IsLast, c.Time, c.AreaName, c.LatLng, c.Depth, c.Magnitude, c.Intensity, c.Url)
}
