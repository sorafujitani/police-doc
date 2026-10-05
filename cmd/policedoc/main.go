package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/sorafujitani/police-doc/internal/app"
)

var version = "dev"

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	code := app.Run(ctx, os.Args[1:], os.Stdout, os.Stderr, version)
	cancel()
	os.Exit(code)
}
