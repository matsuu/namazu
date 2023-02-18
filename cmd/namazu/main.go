package main

import (
	"flag"
	"os"

	"github.com/matsuu/namazu/eew"
	"golang.org/x/exp/slog"
)

func main() {
	var logger = slog.New(slog.NewTextHandler(os.Stderr))
	slog.SetDefault(logger)

	var apiKey string
	flag.StringVar(&apiKey, "apikey", "", "API Key for dmdata.jp")

	var zmqEndpoint string
	flag.StringVar(&zmqEndpoint, "zmq", eew.DefaultZmqEndpoint, "zeromq endpoint")

	flag.Parse()

	if apiKey == "" {
		apiKey = os.Getenv("DMDATA_API_KEY")
	}

	if zmqEndpoint == eew.DefaultZmqEndpoint {
		if v := os.Getenv("ZMQ_ENDPOINT"); v != "" {
			zmqEndpoint = os.Getenv("ZMQ_ENDPOINT")
		}
	}

	for {
		err := eew.Run(apiKey, zmqEndpoint)
		if err != nil {
			slog.Error("Failed to run", err)
			break
		}
		slog.Info("Reconnect...")
	}
}
