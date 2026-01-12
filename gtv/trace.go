package gtv

import (
	"io"
	"runtime/trace"
	"sync"
	"sync/atomic"
)

var (
	started   atomic.Bool
	stopOnce  sync.Once
	flushOnce sync.Once
	flushFn   atomic.Value // func() error
	closeFn   atomic.Value // io.Closer
)

// Start begins runtime tracing and records a flush/close hook for later.
func Start(w io.Writer) error {
	if err := trace.Start(w); err != nil {
		return err
	}
	started.Store(true)
	if c, ok := w.(io.Closer); ok {
		closeFn.Store(c)
	}
	return nil
}

// SetFlushFunc registers a synchronous flush routine (overrides any stored closer).
func SetFlushFunc(fn func() error) {
	if fn == nil {
		return
	}
	flushFn.Store(fn)
}

// Stop ends runtime tracing (idempotent).
func Stop() {
	stopOnce.Do(func() {
		if started.Load() {
			trace.Stop()
		}
	})
}

// Flush runs the configured flush routine or closes the trace writer (idempotent).
func Flush() {
	flushOnce.Do(func() {
		if v := flushFn.Load(); v != nil {
			_ = v.(func() error)()
			return
		}
		if v := closeFn.Load(); v != nil {
			_ = v.(io.Closer).Close()
		}
	})
}
