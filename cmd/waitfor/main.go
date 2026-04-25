package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/pbsladek/wait-for/internal/cli"
)

func main() {
	signals := terminationSignals()

	// Register a buffered channel so neither signal is dropped while Execute runs.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, signals...)
	defer signal.Stop(sigCh)

	ignoreBrokenPipe()

	ctx, stop := signal.NotifyContext(context.Background(), signals...)
	defer stop()

	code := cli.Execute(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)

	select {
	case sig := <-sigCh:
		exitAfterSignal(sig)
	default:
		os.Exit(code)
	}
}
