package nostr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/go-zeromq/zmq4"
	"github.com/matsuu/namazu/eew"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	"golang.org/x/exp/slog"
)

const (
	ZmqSubscribeType = "VXSE45"
	ZmqRelayEndpoint = "tcp://127.0.0.1:5564"
)

var defaultRelays = []string{
	// "ws://127.0.0.1:7001",

	"wss://relay.nostr.wirednet.jp",
	"wss://relay-jp.nostr.wirednet.jp",
	"wss://nostr.h3z.jp",
	"wss://nostr-relay.nokotaro.com",
	"wss://nostr.holybea.com",
	"wss://relay.nostr.or.jp",

	"wss://relay.snort.social",
	"wss://eden.nostr.land",
	"wss://atlas.nostr.land",
	"wss://relay.damus.io",
}

type Event struct {
	XmlId     string
	Serial    int
	Message   string
	NostrId   string
	RootId    string
	ExpiresAt time.Time
}

func getRelays(ctx context.Context, npub string) ([]string, error) {

	var relay *nostr.Relay
	var err error

	// 当該ユーザのリレー情報がないかdefaultRelaysから確認する
	for _, url := range defaultRelays {
		relay, err = nostr.RelayConnect(ctx, url)
		if err != nil {
			slog.Error("Failed to connect relay. try next...", err, slog.Any("relay", url))
			continue
		}
		break
	}
	if relay == nil {
		slog.Error("Failed to connect all default relays", err)
		return nil, err
	}
	defer relay.Close()

	// 直近30日から探す
	since := time.Now().Add(-30 * 24 * time.Hour)

	// NIP-65
	filters := []nostr.Filter{{
		// KindRecommendServer
		Kinds:   []int{2, 10002},
		Authors: []string{npub},
		Since:   &since,
		Limit:   10,
	}}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	sub := relay.Subscribe(ctx, filters)
	var ev *nostr.Event
E:
	// 最後に取得したevを利用する
	for {
		var ok bool
		select {
		case ev, ok = <-sub.Events:
			if !ok {
				break E
			}
			slog.Debug("Got ev", slog.Any("event", ev))
		case <-sub.EndOfStoredEvents:
			slog.Debug("Got EndOfStoredEvents")
			break E
		}
	}
	if ev == nil {
		return nil, fmt.Errorf("no ev")
	}

	// readのみの権限は除外
	var relays []string
	for _, tag := range ev.Tags {
		if tag[0] != "r" {
			continue
		}
		if len(tag) > 2 && tag[2] == "read" {
			slog.Info("Skip readonly channel", slog.Any("tag", tag))
			continue
		}
		relay := tag[1]
		relays = append(relays, relay)
	}

	return relays, nil
}

func Run(ctx context.Context, nsec, zmqEndpoint string) error {
	var sk string
	if nsec != "" {
		if _, s, err := nip19.Decode(nsec); err != nil {
			return err
		} else {
			sk = s.(string)
		}
	} else {
		sk = nostr.GeneratePrivateKey()
		slog.Warn("no secret key. Generated", slog.Any("sec", sk))
	}

	sub := zmq4.NewSub(ctx, zmq4.WithAutomaticReconnect(true), zmq4.WithID(zmq4.SocketIdentity("nostr")))
	defer sub.Close()
	if err := sub.Dial(zmqEndpoint); err != nil {
		slog.Error("Failed to dial zmq namazu", err)
		return err
	}
	if err := sub.SetOption(zmq4.OptionSubscribe, ZmqSubscribeType); err != nil {
		slog.Error("Failed to set option for zmq namazu", err)
		return err
	}
	slog.Info("Succeed to dial zmq namazu", slog.Any("endpoint", ZmqRelayEndpoint))

	relaysPub := zmq4.NewPub(ctx)
	defer relaysPub.Close()
	if err := relaysPub.Listen(ZmqRelayEndpoint); err != nil {
		slog.Error("Failed to listen relay pubsub", err)
		return err
	}
	slog.Info("Succeed to listen zmq relay", slog.Any("endpoint", ZmqRelayEndpoint))

	if err := relayWorker(ctx, sk); err != nil {
		slog.Error("Failed to run relayWorker", err)
		return err
	}

	if err := eventWorker(ctx, sub, relaysPub, sk); err != nil {
		slog.Error("Failed to run eventWorker", err)
	}

	return nil
}

func eventWorker(ctx context.Context, sub, relaysPub zmq4.Socket, sk string) error {
	pub, err := nostr.GetPublicKey(sk)
	if err != nil {
		return err
	}

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
		slog.Info("Wait receive from pubsub")
		msg, err := sub.Recv()
		if err != nil {
			slog.Error("Failed to receive from pubsub", err)
			return err
		}
		slog.Info("Succeed to receive from pubsub", slog.Any("msg", msg))

		content, err := eew.NewContent(bytes.NewReader(msg.Frames[1]))
		if err != nil {
			slog.Error("Failed to parse xml", err)
			return err
		}
		ev := Event{
			XmlId:   content.EventId,
			Serial:  int(content.Serial),
			Message: content.String(),
		}

		var tags nostr.Tags
		if v, ok := eventMap.Load(ev.XmlId); ok {
			prev := v.(Event)
			// 過去報もしくは同じものが届いた場合はスキップ
			if ev.Serial <= prev.Serial {
				slog.Info("Skip old serial", slog.Any("now", ev), slog.Any("prev", prev))
				continue
			}
			if prev.RootId == "" {
				// 空になっているのはおかしいので警告
				slog.Warn("Failed to get RootId", slog.Any("event", ev))
			} else {
				// rootイベントは引き継ぐ
				ev.RootId = prev.RootId
				tag := nostr.Tag{"e", ev.RootId, "", "root"}
				tags = append(tags, tag)
				// 1つ前のがrootと異なるならreplyとして追加
				if prev.RootId != prev.NostrId {
					tag := nostr.Tag{"e", prev.NostrId, "", "reply"}
					tags = append(tags, tag)
				}
				// rootもreplyも自分自身
				tag = nostr.Tag{"p", pub}
				tags = append(tags, tag)
			}
		}
		e := nostr.Event{
			PubKey:    pub,
			CreatedAt: time.Now(),
			Kind:      1,
			Tags:      tags,
			Content:   ev.Message,
		}
		e.Sign(sk)

		j, err := json.Marshal(e)
		if err != nil {
			slog.Error("Failed to Marshal event", err, slog.Any("event", e))
			return err
		}
		t := []byte(msg.Frames[0])
		msg = zmq4.NewMsgFrom(t, j)
		if err := relaysPub.Send(msg); err != nil {
			slog.Error("Failed to send msg to relaysPub", err, slog.Any("type", t), slog.Any("msg", msg))
		}
		slog.Info("Succeed to send msg to relaysPub", slog.Any("type", t), slog.Any("msg", msg))

		// EventIDはreplyに使われるので記録しておく
		ev.NostrId = e.ID
		// rootが設定されていなければ自分がrootになる
		if ev.RootId == "" {
			ev.RootId = e.ID
		}
		eventMap.Store(ev.XmlId, ev)
	}
}

func relayWorker(ctx context.Context, sk string) error {
	pub, err := nostr.GetPublicKey(sk)
	if err != nil {
		return err
	}

	relays, err := getRelays(ctx, pub)
	if err != nil || len(relays) == 0 {
		slog.Error("use default relays because it fails to get your relays", err)
		relays = defaultRelays
	}

	for _, url := range relays {
		ch := make(chan nostr.Event)

		go func(ctx context.Context, url string, ch chan<- nostr.Event) {
			defer close(ch)
			sub := zmq4.NewSub(ctx, zmq4.WithAutomaticReconnect(true), zmq4.WithID(zmq4.SocketIdentity(url)))
			defer sub.Close()
			if err := sub.Dial(ZmqRelayEndpoint); err != nil {
				slog.Error("Failed to dial zmq4 pubsub", err)
				return
			}
			if err := sub.SetOption(zmq4.OptionSubscribe, ZmqSubscribeType); err != nil {
				slog.Error("Failed to set option for subscribe", err)
				return
			}
			slog.Info("Succeed to subscirbe from relaysPub", slog.Any("endpoint", ZmqRelayEndpoint))
			for {
				msg, err := sub.Recv()
				if err != nil {
					slog.Error("Failed to receive msg", err)
					return
				}
				if msg.Frames == nil || len(msg.Frames) != 2 {
					slog.Warn("received msg has unexpected frames", slog.Any("msg", msg))
					continue
				}
				ev := nostr.Event{}
				if err := json.Unmarshal(msg.Frames[1], &ev); err != nil {
					slog.Error("Failed to unmarshal event", err, slog.Any("msg", msg))
					continue
				}
				ch <- ev
			}
		}(ctx, url, ch)

		go func(ctx context.Context, url string, ch <-chan nostr.Event) {
			attempt := 1
			for {
				relay, err := nostr.RelayConnect(ctx, url)
				if err != nil {
					slog.Error("failed to connect. Try next...", err, slog.Any("relay", url))
					if attempt < 3600 {
						attempt *= 2
					}
					time.Sleep(time.Duration(attempt) * time.Second)
					continue
				}
				slog.Info("Succeed to connect to relay", slog.Any("relay", url))
				attempt = 1

			LOOP:
				for {
					select {
					case ev, ok := <-ch:
						if !ok {
							relay.Close()
							return
						}
						status := relay.Publish(ctx, ev)
						if status == nostr.PublishStatusFailed {
							slog.Warn("Failed to publish event to relay", slog.Any("event", ev), slog.Any("relay", url), slog.Any("status", status))
						} else {
							slog.Info("Succeed to publish event to relay", slog.Any("event", ev), slog.Any("relay", url), slog.Any("status", status))
						}
					case err := <-relay.ConnectionError:
						slog.Error("Connection error. retry...", err, slog.Any("relay", url))
						time.Sleep(10 * time.Second)
						break LOOP
					}
				}
				relay.Close()
			}
		}(ctx, url, ch)
	}

	return nil
}
