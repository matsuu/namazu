package main

import (
	"context"
	"flag"
	"os"

	"github.com/matsuu/namazu/eew"
	"github.com/matsuu/namazu/mixi2"
	"golang.org/x/exp/slog"
)

func main() {
	var zmqEndpoint string
	flag.StringVar(&zmqEndpoint, "zmq", eew.DefaultZmqEndpoint, "connect to namazu endpoint")

	var authKey string
	flag.StringVar(&authKey, "auth-key", "", "auth key for mixi2")

	var authToken string
	flag.StringVar(&authToken, "auth-token", "", "auth token for mixi2")

	var userAgent string
	flag.StringVar(&userAgent, "user-agent", "", "User-Agent for mixi2")

	flag.Parse()

	if zmqEndpoint == eew.DefaultZmqEndpoint {
		if v := os.Getenv("ZMQ_ENDPOINT"); v != "" {
			zmqEndpoint = os.Getenv("ZMQ_ENDPOINT")
		}
	}

	if authKey == "" {
		authKey = os.Getenv("MIXI2_AUTH_KEY")
	}

	if authToken == "" {
		authToken = os.Getenv("MIXI2_AUTH_TOKEN")
	}

	if userAgent == "" {
		userAgent = os.Getenv("MIXI2_USER_AGENT")
	}

	ctx := context.Background()
	if err := mixi2.Run(ctx, zmqEndpoint, authKey, authToken, userAgent); err != nil {
		slog.Error("failed to send to mstdn", err)
		os.Exit(1)
	}
}
