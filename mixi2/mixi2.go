package mixi2

import (
	"bytes"
	"context"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/go-zeromq/zmq4"
	"github.com/matsuu/go-mixi2"
	"github.com/matsuu/go-mixi2/gen/com/mixi/mercury/api"
	"github.com/matsuu/namazu/eew"
	"golang.org/x/exp/slog"
)

const (
	ZmqSubscribeType = "VXSE45"
)

type Event struct {
	XmlId     string
	Serial    int
	Message   string
	ExpiresAt time.Time
	PostId    string
}

func Run(ctx context.Context, zmqEndpoint, authKey, authToken, userAgent string) error {
	sub := zmq4.NewSub(ctx, zmq4.WithAutomaticReconnect(true))
	defer sub.Close()
	if err := sub.Dial(zmqEndpoint); err != nil {
		slog.Error("Failed to dial zmq4 pubsub", err, slog.Any("zmq", zmqEndpoint))
		return err
	}
	if err := sub.SetOption(zmq4.OptionSubscribe, ZmqSubscribeType); err != nil {
		slog.Error("Failed to set option for subscribe", err)
		return err
	}

	c := mixi2.NewClient(mixi2.WithAuth(authKey, authToken, userAgent))

	var eventMap sync.Map
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for now := range ticker.C {
			eventMap.Range(func(k, v any) bool {
				if v.(Event).ExpiresAt.After(now) {
					eventMap.Delete(k)
				}
				return true
			})
		}
	}()

	for {
		msg, err := sub.Recv()
		if err != nil {
			slog.Error("Failed to receive from pubsub", err)
			break
		}
		slog.Info("Succeed to receive from pubsub", slog.Any("msg", msg))

		content, err := eew.NewContent(bytes.NewReader(msg.Frames[1]))
		if err != nil {
			slog.Error("Failed to parse xml", err)
			continue
		}
		ev := Event{
			XmlId:   content.EventId,
			Serial:  int(content.Serial),
			Message: content.String(),
		}

		post := &api.CreatePostRequest{
			Text: ev.Message,
		}
		if v, ok := eventMap.Load(ev.XmlId); ok {
			prev := v.(Event)
			// 過去報もしくは同じものが届いた場合はスキップ
			if ev.Serial <= prev.Serial {
				slog.Info("skip old serial", slog.Any("now", ev), slog.Any("prev", prev))
				continue
			}
		}
		// mixi2はリプライをすると何度も現れることになるため最終報のみ送信する
		if !content.IsLast {
			continue
		}
		resp, err := c.CreatePost(ctx, connect.NewRequest(post))
		if err != nil {
			slog.Error("failed to create post", err, slog.Any("post", post))
			return err
		}
		slog.Info("Succeed to post status", slog.Any("resp", resp), slog.Any("post", post))
		ev.PostId = resp.Msg.Post.PostId
		eventMap.Store(ev.XmlId, ev)
	}
	return nil
}
