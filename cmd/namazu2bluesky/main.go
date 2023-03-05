package main

import (
	"context"
	"flag"
	"os"

	"github.com/matsuu/namazu/bluesky"
	"github.com/matsuu/namazu/eew"
	"golang.org/x/exp/slog"
)

func main() {
	var zmqEndpoint string
	flag.StringVar(&zmqEndpoint, "zmq", eew.DefaultZmqEndpoint, "connect to namazu endpoint")

	var pdsUrl string
	flag.StringVar(&pdsUrl, "pds-host", "https://bsky.social", "method, hostname, and port of PDS instance")

	var authFile string
	flag.StringVar(&authFile, "auth-file", "bsky.auth", "path to JSON file with ATP auth info")

	flag.Parse()

	if zmqEndpoint == eew.DefaultZmqEndpoint {
		if v := os.Getenv("ZMQ_ENDPOINT"); v != "" {
			zmqEndpoint = os.Getenv("ZMQ_ENDPOINT")
		}
	}

	if pdsUrl == "" {
		pdsUrl = os.Getenv("ATP_PDS_HOST")
	}

	if authFile == "" {
		authFile = os.Getenv("ATP_AUTH_FILE")
	}

	ctx := context.Background()
	if err := bluesky.Run(ctx, zmqEndpoint, pdsUrl, authFile); err != nil {
		slog.Error("Failed to send to bluesky", err)
	}
}
