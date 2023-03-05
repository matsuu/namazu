package bluesky

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	comatproto "github.com/bluesky-social/indigo/api/atproto"
	appbsky "github.com/bluesky-social/indigo/api/bsky"
	lexutil "github.com/bluesky-social/indigo/lex/util"
	"github.com/bluesky-social/indigo/xrpc"
	"github.com/go-zeromq/zmq4"
	"github.com/matsuu/namazu/eew"
	"golang.org/x/exp/slog"
)

const (
	ZmqSubscribeType = "VXSE45"
)

type Event struct {
	XmlId       string
	Serial      int
	Message     string
	FeedRef     *comatproto.RepoStrongRef
	RootFeedRef *comatproto.RepoStrongRef
	ExpiresAt   time.Time
}

func getXrpcClient(host string, authInfo *xrpc.AuthInfo) *xrpc.Client {
	xrpcc := xrpc.Client{
		Client: http.DefaultClient,
		Host:   host,
		Auth:   authInfo,
	}
	return &xrpcc

}

func createSessionByPassword(ctx context.Context, host, handle, password string) (*comatproto.SessionCreate_Output, error) {
	xrpcc := getXrpcClient(host, nil)
	ses, err := comatproto.SessionCreate(ctx, xrpcc, &comatproto.SessionCreate_Input{
		Identifier: &handle,
		Password:   password,
	})
	return ses, err
}

func createSession(ctx context.Context, host, authFile string) (*xrpc.Client, error) {
	var authInfo xrpc.AuthInfo
	in, err := os.Open(authFile)
	if err != nil {
		return nil, err
	}
	err = json.NewDecoder(in).Decode(&authInfo)
	in.Close()
	if err != nil {
		return nil, err
	}
	xrpcc := getXrpcClient(host, &authInfo)
	ses, err := comatproto.SessionGet(ctx, xrpcc)
	if err != nil {
		slog.Error("Failed to get session", err)
		return nil, err
	}
	slog.Info("Succeed to session get", slog.Any("session", ses))

	return xrpcc, nil
}

func refreshSession(ctx context.Context, host, authFile string) (*xrpc.Client, error) {
	var authInfo xrpc.AuthInfo
	in, err := os.Open(authFile)
	if err != nil {
		return nil, err
	}
	err = json.NewDecoder(in).Decode(&authInfo)
	in.Close()
	if err != nil {
		return nil, err
	}

	xrpcc := getXrpcClient(host, &authInfo)

	ses, err := comatproto.SessionRefresh(ctx, xrpcc)
	if err != nil {
		slog.Error("Failed to session refresh", err, slog.Any("authInfo", authInfo))
		return nil, err
	}
	out, err := os.Create(authFile)
	if err != nil {
		return nil, err
	}
	err = json.NewEncoder(out).Encode(ses)
	out.Close()
	if err != nil {
		return nil, err
	}
	// 旧authInfoは古いので作り直す
	xrpcc, err = createSession(ctx, host, authFile)
	return xrpcc, err
}

func Run(ctx context.Context, zmqEndpoint, pdsUrl, authFile string) error {
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

	xrpcc, err := createSession(ctx, pdsUrl, authFile)
	// xrpcc, err := refreshSession(ctx, pdsUrl, authFile)
	if err != nil {
		slog.Error("Failed to create session", err)
		return err
	}
	slog.Info("Succeed to create session")

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

	ch := make(chan Event, 10)
	// pubsubを受けてchannelに流す
	go func(ch chan<- Event) {
		defer close(ch)
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
			ch <- ev
		}
	}(ch)

	// ticker := time.NewTicker(time.Hour)
	// defer ticker.Stop()
LOOP:
	for {
		select {
		// case <-ticker.C:
		// 	var err error
		// 	xrpcc, err = refreshSession(ctx, pdsUrl, authFile)
		// 	if err != nil {
		// 		return err
		// 	}
		case ev, ok := <-ch:
			if !ok {
				slog.Warn("closed channel")
				break LOOP
			}
			var reply *appbsky.FeedPost_ReplyRef
			if v, ok := eventMap.Load(ev.XmlId); ok {
				prev := v.(Event)
				// 過去報もしくは同じものが届いた場合はスキップ
				if ev.Serial <= prev.Serial {
					slog.Info("Skip old serial", slog.Any("now", ev), slog.Any("prev", prev))
					continue
				}
				if prev.RootFeedRef == nil {
					// 空になっているのはおかしいので警告
					slog.Warn("Failed to get Root", slog.Any("event", ev))
				} else {
					reply = &appbsky.FeedPost_ReplyRef{
						Parent: prev.FeedRef,
						Root:   prev.RootFeedRef,
					}
					// rootイベントは引き継ぐ
					ev.RootFeedRef = prev.RootFeedRef
				}
			}
			record := comatproto.RepoCreateRecord_Input{
				Collection: "app.bsky.feed.post",
				Did:        xrpcc.Auth.Did,
				Record: lexutil.LexiconTypeDecoder{
					Val: &appbsky.FeedPost{
						Text:      ev.Message,
						CreatedAt: time.Now().Format("2006-01-02T15:04:05.000Z"),
						Reply:     reply,
					},
				},
			}

			resp, err := comatproto.RepoCreateRecord(ctx, xrpcc, &record)
			if err != nil {
				return fmt.Errorf("failed to create record: %w", err)
			}
			slog.Info("Succeed to post record", slog.Any("record", record))
			// CidとUriはreplyに使われるので記録しておく
			ev.FeedRef = &comatproto.RepoStrongRef{
				Cid: resp.Cid,
				Uri: resp.Uri,
			}
			eventMap.Store(ev.XmlId, ev)
		}
	}
	return nil
}
