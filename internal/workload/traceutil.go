package workload

import (
	"context"
	"fmt"
	"os"
	"runtime/trace"
	"strings"
)

// TraceSend wraps a channel send in a region and emits a value log when tracing is enabled.
// The region label should be a human-friendly, stable string (e.g., "server: send to clientout[0]").
func TraceSend[T any](ctx context.Context, label string, ch chan<- T, v T) {
	trace.WithRegion(ctx, label, func() {
		ch <- v
		if trace.IsEnabled() && valueLogsEnabled() {
			trace.Log(ctx, "value", fmt.Sprint(v))
		}
	})
}

// TraceRecv wraps a channel receive in a region and emits a value log when tracing is enabled.
// It returns the received value.
func TraceRecv[T any](ctx context.Context, label string, ch <-chan T) T {
	var v T
	trace.WithRegion(ctx, label, func() {
		v = <-ch
		if trace.IsEnabled() && valueLogsEnabled() {
			trace.Log(ctx, "value", fmt.Sprint(v))
		}
	})
	return v
}

func valueLogsEnabled() bool {
	v := strings.TrimSpace(os.Getenv("GTV_LOG_VALUES"))
	if v == "" {
		return false
	}
	v = strings.ToLower(v)
	return v == "1" || v == "true" || v == "yes" || v == "on"
}
