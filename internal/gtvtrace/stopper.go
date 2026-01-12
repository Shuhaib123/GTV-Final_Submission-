package gtvtrace

import (
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

// StopFlushExit stops tracing, flushes output, and exits with the given code.
// Note: os.Exit skips defers, so Flush must be synchronous.
func StopFlushExit(exitCode int) {
	StopIdempotent()
	_ = Flush()
	os.Exit(exitCode)
}

// InstallStopOnSignal stops tracing on SIGINT/SIGTERM.
func InstallStopOnSignal() {
	ch := make(chan os.Signal, 2)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ch
		StopFlushExit(0)
	}()
}

// InstallStopAfterFromEnv stops tracing after a timeout in milliseconds from envVar.
func InstallStopAfterFromEnv(envVar string) {
	v := os.Getenv(envVar)
	if v == "" {
		return
	}
	ms, err := strconv.Atoi(v)
	if err != nil || ms <= 0 {
		return
	}
	go func() {
		time.Sleep(time.Duration(ms) * time.Millisecond)
		StopFlushExit(0)
	}()
}
