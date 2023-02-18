package eew

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-zeromq/zmq4"
	"golang.org/x/exp/slog"
	"golang.org/x/net/websocket"
)

const (
	socketUrl      = "https://api.dmdata.jp/v2/socket"
	classification = "eew.forecast"
	contentType    = "application/json"
)

type SocketRequest struct {
	Classifications []string `json:"classifications"`
	Types           []string `json:"types,omitempty"`
	Test            string   `json:"test,omitempty"`
	AppName         string   `json:"appName,omitempty"`
	FormatMode      string   `json:"formatMode,omitempty"`
}

type SocketResponseStatus struct {
	ResponseId   string    `json:"responseId"`
	ResponseTime time.Time `json:"responseTime"`
	Status       string    `json:"status"`
}

type SocketResponse struct {
	SocketResponseStatus
	Ticket    string `json:"ticket"`
	Websocket struct {
		Id         int      `json:"id"`
		Url        string   `json:"url"`
		Protocol   []string `json:"protocol"`
		Expiration int      `json:"expiration"`
	} `json:"websocket"`
	Classifications []string `json:"classifications"`
	Test            string   `json:"test"`
	Types           []string `json:"types"`
	Formats         []string `json:"formats"`
	AppName         *string  `json:"appName"`
}

type SocketResponseError struct {
	SocketResponseStatus
	Error struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error"`
}

type WebsocketType struct {
	Type string `json:"type"`
}

type WebsocketStart struct {
	WebsocketType
	SocketId        int       `json:"socketId"`
	Classifications []string  `json:"Classifications"`
	Types           []string  `json:"types"`
	Test            string    `json:"test"`
	Formats         []string  `json:"formats"`
	AppName         *string   `json:"appName"`
	Time            time.Time `json:"time"`
}

type WebsocketPing struct {
	WebsocketType
	PingId string `json:"pingId"`
}

type WebsocketData struct {
	WebsocketType
	Version        string `json:"version"`
	Id             string `json:"id"`
	Classification string `json:"classification"`
	Passing        []struct {
		Name string    `json:"name"`
		Time time.Time `json:"time"`
	} `json:"passing"`
	Head struct {
		Type        string    `json:"type"`
		Author      string    `json:"author"`
		Target      string    `json:"target"`
		Time        time.Time `json:"time"`
		Designation *string   `json:"designation"`
		Test        bool      `json:"test"`
		Xml         bool      `json:"xml"`
	} `json:"head"`
	XmlReport   any     `json:"xmlReport"`
	Format      *string `json:"format"`
	Compression *string `json:"compression"`
	Encoding    *string `json:"encoding"`
	Body        string  `json:"body"`
}

type WebsocketError struct {
	WebsocketType
	Error string `json:"error"`
	Code  int    `json:"code"`
	Close bool   `json:"close"`
}

func ParseSocketResponse(resp io.Reader) (*SocketResponse, error) {
	body, err := io.ReadAll(resp)
	if err != nil {
		return nil, err
	}
	buf := bytes.NewReader(body)
	var s SocketResponseStatus
	if err := json.NewDecoder(buf).Decode(&s); err != nil {
		return nil, err
	}
	switch s.Status {
	case "ok":
		var s SocketResponse
		buf.Seek(0, 0)
		err := json.NewDecoder(buf).Decode(&s)
		return &s, err
	case "error":
		var s SocketResponseError
		buf.Seek(0, 0)
		err := json.NewDecoder(buf).Decode(&s)
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("Error code:%d message:%s", s.Error.Code, s.Error.Message)
	}
	return nil, fmt.Errorf("unknown response: %v", s)
}

func Run(apiKey string) error {
	if apiKey == "" {
		return fmt.Errorf("no api key")
	}
	sreq := SocketRequest{
		Classifications: []string{classification},
		Test:            "including",
	}
	j, err := json.Marshal(sreq)
	if err != nil {
		slog.Error("Failed to marshal SocketRequest", err, slog.Any("sreq", sreq))
		return err
	}

	u, err := url.Parse(socketUrl)
	if err != nil {
		slog.Error("Failed to parse socket url", err, slog.Any("url", socketUrl))
		return err
	}
	q := u.Query()
	q.Set("key", apiKey)
	u.RawQuery = q.Encode()
	slog.Info("Connect to get socketStart", slog.Any("url", u), slog.Any("contentType", contentType), slog.Any("json", j))
	resp, err := http.Post(u.String(), contentType, bytes.NewReader(j))
	if err != nil {
		slog.Error("Failed to post SocketRequest", err)
		return err
	}
	defer resp.Body.Close()
	sres, err := ParseSocketResponse(resp.Body)
	if err != nil {
		slog.Error("Failed to parse SocketResponse", err)
		return err
	}
	slog.Info("Succeed to get response", slog.Any("sres", sres))
	defer func(id int) {
		socketCloseUrl := fmt.Sprintf("%s/%d", socketUrl, id)
		req, err := http.NewRequest("DELETE", socketCloseUrl, nil)
		if err != nil {
			slog.Error("Failed to generate request for socketClose", err, slog.Any("url", socketCloseUrl))
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			slog.Error("Failed to close socket", err, slog.Any("url", socketCloseUrl))
		}
		res.Body.Close()
	}(sres.Websocket.Id)

	wsUrl := sres.Websocket.Url
	protocol := sres.Websocket.Protocol[0]
	origin := "http://localhost"
	slog.Info("Connect to websocket", slog.Any("url", wsUrl), slog.Any("protocol", protocol), slog.Any("origin", origin))
	ws, err := websocket.Dial(wsUrl, protocol, origin)
	if err != nil {
		slog.Error("Failed to dial websocket", err)
		return err
	}
	defer ws.Close()
	slog.Info("Succeed to connect websocket", slog.Any("ws", ws))

	ctx := context.Background()
	pub := zmq4.NewPub(ctx)
	defer pub.Close()
	if err := pub.Listen(ZmqEndpoint); err != nil {
		slog.Error("Failed to listen zmq4 pubsub", err)
		return err
	}

	for {
		var b []byte
		if err := websocket.Message.Receive(ws, &b); err != nil {
			slog.Error("Failed to receive websocket message", err)
			// 閉じられたと判断して再接続
			break
		}
		var checkType WebsocketType
		if err := json.Unmarshal(b, &checkType); err != nil {
			slog.Error("Failed to unmarshal received websocket message as checkType", err, slog.Any("received", string(b)), slog.Any("length", len(b)))
			continue
		}
		if checkType.Type != "ping" {
			slog.Info("Received websocket message", slog.Any("received", string(b)), slog.Any("length", len(b)))
		}
		switch checkType.Type {
		case "start":
			var start WebsocketStart
			if err := json.Unmarshal(b, &start); err != nil {
				slog.Error("Failed to unmarshal start message", err)
				continue
			}
			slog.Info("Succeed to start websocket", slog.Any("start", start))
		case "ping":
			var ping WebsocketPing
			if err := json.Unmarshal(b, &ping); err != nil {
				slog.Error("Failed to unmarshal ping message", err)
				continue
			}
			pong := ping
			pong.Type = "pong"
			if err := websocket.JSON.Send(ws, pong); err != nil {
				slog.Error("Failed to send pong message", err, slog.Any("pong", pong))
			}
			slog.Debug("Succeed to send pong", slog.Any("pong", pong))
		case "data":
			var data WebsocketData
			if err := json.Unmarshal(b, &data); err != nil {
				slog.Error("Failed to unmarshal data message", err)
				continue
			}
			var r io.Reader
			switch *data.Encoding {
			case "base64":
				b, err := base64.StdEncoding.DecodeString(data.Body)
				if err != nil {
					slog.Error("Failed to decode base64 data.Body", err, slog.Any("base64", data.Body))
					continue
				}
				r = bytes.NewReader(b)
			case "utf-8":
				r = strings.NewReader(data.Body)
			default:
				slog.Warn("unknown encoding", slog.Any("encoding", data.Encoding))
				continue
			}
			switch *data.Compression {
			case "gzip":
				var err error
				r, err = gzip.NewReader(r)
				if err != nil {
					slog.Error("Failed to uncompress gzip data", err)
					continue
				}
			case "zip":
				slog.Warn("zip is not supported", slog.Any("compression", data.Compression))
				continue
			}
			x, err := io.ReadAll(r)
			if err != nil {
				slog.Error("Failed to ReadAll xml", err)
			}
			msg := zmq4.NewMsgFrom([]byte(data.Head.Type), x)
			if err := pub.Send(msg); err != nil {
				slog.Error("Failed to send zmq", err)
				continue
			}
			slog.Info("Succeed to send zmq")
		case "error":
			var e WebsocketError
			if err := json.Unmarshal(b, &e); err != nil {
				slog.Error("Failed to unmarshal ping message", err)
				continue
			}
			slog.Warn("Received error message", slog.Any("error", e))
			if e.Close {
				return nil
			}
		default:
			slog.Warn("unknown type", slog.Any("type", checkType.Type))
		}
	}
	return nil
}
