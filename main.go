package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/kamuridesu/unrestrict-bot/internal/app"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cmd := "run"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	var err error
	switch cmd {
	case "run":
		err = app.Run(ctx)
	case "gen-session", "gensession", "session":
		err = app.GenSession(ctx)
	case "forward":
		err = app.Forward(ctx)
	case "healthcheck":
		err = app.HealthCheck(ctx)
	case "-h", "--help", "help":
		usage()
		return
	default:
		usage()
		os.Exit(2)
	}

	if err != nil && ctx.Err() == nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `unrestrict-bot - save media from restricted Telegram chats

Usage:
  unrestrict-bot [run]         Start the service (default).
  unrestrict-bot gen-session   Authenticate and print SESSION_STRING.
  unrestrict-bot forward       Bulk-forward media FORWARD_FROM -> FORWARD_TO.

Configuration is read from the environment. See README.md.
`)
}
