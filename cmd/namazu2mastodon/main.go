package main

import (
	"context"
	"os"

	"github.com/matsuu/namazu/mastodon"
	"golang.org/x/exp/slog"
)

func main() {
	var logger = slog.New(slog.NewTextHandler(os.Stderr))
	slog.SetDefault(logger)

	zmqEndpoint := os.Getenv("ZMQ_ENDPOINT")
	mstdnServer := os.Getenv("MSTDN_SERVER")
	clientId := os.Getenv("MSTDN_CLIENT_ID")
	clientSecret := os.Getenv("MSTDN_CLIENT_SECRET")
	accessToken := os.Getenv("MSTDN_ACCESS_TOKEN")

	ctx := context.Background()
	if err := mastodon.Run(ctx, zmqEndpoint, mstdnServer, clientId, clientSecret, accessToken); err != nil {
		slog.Error("Failed to send to mstdn", err)
	}
}
