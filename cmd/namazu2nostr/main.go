package main

import (
	"context"
	"os"

	"github.com/matsuu/namazu/nostr"
	"golang.org/x/exp/slog"
)

var logger = slog.New(slog.NewTextHandler(os.Stderr))

func main() {

	nsec := os.Getenv("NOSTR_SECRET_KEY")

	ctx := context.Background()
	if err := nostr.Run(ctx, nsec); err != nil {
		logger.Error("Failed to send to nostr", err)
	}
}
