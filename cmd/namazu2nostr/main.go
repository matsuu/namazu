package main

import (
	"context"
	"flag"
	"os"

	"github.com/matsuu/namazu/eew"
	"github.com/matsuu/namazu/nostr"
	"golang.org/x/exp/slog"
)

var logger = slog.New(slog.NewTextHandler(os.Stderr))

func main() {

	var nsec string
	flag.StringVar(&nsec, "nsec", "", "nsec for nostr")

	var zmqEndpoint string
	flag.StringVar(&zmqEndpoint, "zmq", eew.DefaultZmqEndpoint, "zeromq endpoint")

	flag.Parse()

	if nsec == "" {
		nsec = os.Getenv("NOSTR_SECRET_KEY")
	}

	if zmqEndpoint == eew.DefaultZmqEndpoint {
		if v := os.Getenv("ZMQ_ENDPOINT"); v != "" {
			zmqEndpoint = os.Getenv("ZMQ_ENDPOINT")
		}
	}

	ctx := context.Background()
	if err := nostr.Run(ctx, nsec, zmqEndpoint); err != nil {
		logger.Error("Failed to send to nostr", err)
	}
}
