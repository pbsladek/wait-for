//go:build windows

package main

import (
	"os"
)

func terminationSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}

func ignoreBrokenPipe() {
}

func exitAfterSignal(os.Signal) {
	os.Exit(130)
}
