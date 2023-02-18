package eew

import (
	"os"
	"testing"
)

func TestParseXml(t *testing.T) {
	type data struct {
		File    string
		Message string
	}
	tests := []data{
		{
			File:    "samples/77_01_01_110311_VXSE45.xml",
			Message: "第23報\n11日14時46分ごろ三陸沖（N38.1/E142.9）で最大震度6強（M8.4）の地震が発生。震源の深さは10km。\n#earthquake",
		},
		{
			File:    "samples/77_01_02_110311_VXSE45.xml",
			Message: "第23報\n不明ごろ不明（経緯不明）で最大震度不明（マグニチュード不明）の地震が発生。震源の深さは不明。\n#earthquake",
		},
	}
	for _, d := range tests {
		f, err := os.Open(d.File)
		if err != nil {
			t.Errorf("failed to open sample xml: %v", d.File)
		}
		content, err := NewContent(f)
		f.Close()
		if err != nil {
			t.Errorf("failed to parseXml: %v", err)
		}
		got := content.String()
		want := d.Message
		if got != want {
			t.Errorf("diff got:%s want:%s", got, want)
		}
	}
}
