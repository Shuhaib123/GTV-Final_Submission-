package gtvtrace

import (
	"io"
	"os"
	"runtime/trace"
	"sync"
	"sync/atomic"
)

var (
	stopOnce  sync.Once
	flushOnce sync.Once
	flushErr  error

	mu sync.Mutex

	writer     io.WriteCloser
	ownsWriter bool

	flushHook func() error

	started atomic.Bool
)

// Start begins runtime tracing to the provided writer. The caller owns the writer.
func Start(w io.Writer) error {
	if w == nil {
		err := trace.Start(io.Discard)
		if err == nil {
			started.Store(true)
		}
		return err
	}
	if wc, ok := w.(io.WriteCloser); ok {
		mu.Lock()
		writer = wc
		ownsWriter = false
		mu.Unlock()
	} else {
		mu.Lock()
		writer = nil
		ownsWriter = false
		mu.Unlock()
	}
	err := trace.Start(w)
	if err == nil {
		started.Store(true)
	}
	return err
}

// StartFile creates/owns the output file and begins tracing to it.
func StartFile(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	mu.Lock()
	writer = f
	ownsWriter = true
	mu.Unlock()
	if err := trace.Start(f); err != nil {
		mu.Lock()
		writer = nil
		ownsWriter = false
		mu.Unlock()
		_ = f.Close()
		return err
	}
	started.Store(true)
	return nil
}

// StopIdempotent stops tracing once.
func StopIdempotent() {
	if !started.Load() {
		return
	}
	stopOnce.Do(func() { trace.Stop() })
}

// FlushIdempotent flushes/finishes the trace output once.
func FlushIdempotent() {
	flushOnce.Do(func() {
		mu.Lock()
		w := writer
		owns := ownsWriter
		hook := flushHook
		mu.Unlock()
		if owns && w != nil {
			if f, ok := w.(*os.File); ok {
				_ = f.Sync()
			}
			if err := w.Close(); err != nil && flushErr == nil {
				flushErr = err
			}
		}
		if hook != nil {
			if err := hook(); err != nil && flushErr == nil {
				flushErr = err
			}
		}
	})
}

// Flush returns the error (if any) from the flush hook or writer close.
func Flush() error {
	FlushIdempotent()
	return flushErr
}

// SetFlushHook installs an optional callback invoked during Flush().
func SetFlushHook(fn func() error) {
	mu.Lock()
	defer mu.Unlock()
	flushHook = fn
}
