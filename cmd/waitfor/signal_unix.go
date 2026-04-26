//go:build !windows

package main

import (
	"os"
	"os/signal"
	"syscall"
)

func terminationSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM}
}

func ignoreBrokenPipe() {
	// Suppress SIGPIPE so a broken pipe on stdout/stderr does not crash the
	// process; write errors are already handled by the output path.
	signal.Ignore(syscall.SIGPIPE)
}

func exitAfterSignal(sig os.Signal) {
	// Reset to the default handler and re-raise the signal so parent shells see
	// signal termination rather than a plain numeric exit code.
	signal.Reset(sig)
	if s, ok := sig.(syscall.Signal); ok {
		_ = syscall.Kill(os.Getpid(), s)
	}
	if sig == syscall.SIGTERM {
		os.Exit(143)
	}
	os.Exit(130)
}
