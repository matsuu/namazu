package mastodon

import (
	"bytes"
	"context"
	"sync"
	"time"

	"github.com/go-zeromq/zmq4"
	"github.com/matsuu/namazu/eew"
	"github.com/mattn/go-mastodon"
	"golang.org/x/exp/slog"
)

const (
	ZmqSubscribeType = "VXSE45"
)

type Event struct {
	XmlId     string
	Serial    int
	Message   string
	MstdnId   mastodon.ID
	ExpiresAt time.Time
}

func Run(ctx context.Context, zmqEndpoint, mstdnServer, clientId, clientSecret, accessToken string) error {
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

	c := mastodon.NewClient(&mastodon.Config{
		Server:       mstdnServer,
		ClientID:     clientId,
		ClientSecret: clientSecret,
		AccessToken:  accessToken,
	})
	u, err := c.GetAccountCurrentUser(ctx)
	if err != nil {
		return err
	}
	slog.Info("Succeed to get account info", slog.Any("user", u))

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
		t := mastodon.Toot{
			Status:   ev.Message,
			Language: "ja",
		}
		if v, ok := eventMap.Load(ev.XmlId); ok {
			prev := v.(Event)
			// 過去報もしくは同じものが届いた場合はスキップ
			if ev.Serial <= prev.Serial {
				slog.Info("Skip old serial", slog.Any("now", ev), slog.Any("prev", prev))
				continue
			}
			if prev.MstdnId == "" {
				// 空になっているのはおかしいので警告
				slog.Warn("Failed to get MstdnId", slog.Any("event", ev))
			} else {
				t.InReplyToID = prev.MstdnId
			}
			// 初報は公開、それ以外は未収載
			t.Visibility = mastodon.VisibilityUnlisted
		} else {
			t.Visibility = mastodon.VisibilityPublic
		}

		s, err := c.PostStatus(ctx, &t)
		if err != nil {
			slog.Error("Failed to toot", err, slog.Any("toot", t))
			return err
		}
		slog.Info("Succeed to post status", slog.Any("status", s), slog.Any("toot", t))
		ev.MstdnId = s.ID
		eventMap.Store(ev.XmlId, ev)
	}
	return nil
}
