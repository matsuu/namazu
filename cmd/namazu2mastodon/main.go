package main

import (
	"context"
	"flag"
	"os"

	"github.com/matsuu/namazu/eew"
	"github.com/matsuu/namazu/mastodon"
	"golang.org/x/exp/slog"
)

func main() {
	var logger = slog.New(slog.NewTextHandler(os.Stderr))
	slog.SetDefault(logger)

	var zmqEndpoint string
	flag.StringVar(&zmqEndpoint, "zmq", eew.DefaultZmqEndpoint, "connect to namazu endpoint")

	var mstdnServer string
	flag.StringVar(&mstdnServer, "mstdn", "", "connect to mastodon instance")

	var clientId string
	flag.StringVar(&clientId, "client-id", "", "client id for mastodon")

	var clientSecret string
	flag.StringVar(&clientId, "client-secret", "", "client secret for mastodon")

	var accessToken string
	flag.StringVar(&clientId, "access-token", "", "access token for mastodon")

	flag.Parse()

	if zmqEndpoint == eew.DefaultZmqEndpoint {
		if v := os.Getenv("ZMQ_ENDPOINT"); v != "" {
			zmqEndpoint = os.Getenv("ZMQ_ENDPOINT")
		}
	}

	if mstdnServer == "" {
		mstdnServer = os.Getenv("MSTDN_SERVER")
	}

	if clientId == "" {
		clientId = os.Getenv("MSTDN_CLIENT_ID")
	}

	if clientSecret == "" {
		clientSecret = os.Getenv("MSTDN_CLIENT_SECRET")
	}

	if accessToken == "" {
		accessToken = os.Getenv("MSTDN_ACCESS_TOKEN")
	}

	ctx := context.Background()
	if err := mastodon.Run(ctx, zmqEndpoint, mstdnServer, clientId, clientSecret, accessToken); err != nil {
		slog.Error("Failed to send to mstdn", err)
	}
}
