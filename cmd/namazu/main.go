package main

import (
	"os"

	"github.com/matsuu/namazu/eew"
	"golang.org/x/exp/slog"
)

func main() {
	var logger = slog.New(slog.NewTextHandler(os.Stderr))
	slog.SetDefault(logger)

	apiKey := os.Getenv("DMDATA_API_KEY")

	for {
		err := eew.Run(apiKey)
		if err != nil {
			slog.Error("Failed to run", err)
			break
		}
		slog.Info("Reconnect...")
	}
}
