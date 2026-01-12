package gtv

import (
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

// StopFlushExit stops tracing, flushes synchronously, then exits.
func StopFlushExit(exitCode int) {
	Stop()
	Flush()
	os.Exit(exitCode)
}

// InstallStopOnSignal listens for SIGINT/SIGTERM and flushes before exit.
func InstallStopOnSignal() {
	ch := make(chan os.Signal, 2)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ch
		StopFlushExit(0)
	}()
}

// InstallStopAfterFromEnv sleeps for the env duration, then flushes and exits.
// Accepts either a time.ParseDuration string or a millisecond integer.
func InstallStopAfterFromEnv(varName string) {
	v := os.Getenv(varName)
	if v == "" {
		return
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		if ms, err2 := strconv.Atoi(v); err2 == nil && ms > 0 {
			d = time.Duration(ms) * time.Millisecond
		} else {
			return
		}
	}
	if d <= 0 {
		return
	}
	go func() {
		time.Sleep(d)
		StopFlushExit(0)
	}()
}
