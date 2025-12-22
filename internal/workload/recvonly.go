package workload

import (
	"context"
	"runtime/trace"
)

// RunRecvOnlyProgram performs a single receive from a buffered channel to
// validate that region instrumentation for receives emits symmetric events.
func RunRecvOnlyProgram(ctx context.Context) {
	ch := make(chan int, 1)
	ch <- 42
	trace.WithRegion(ctx, "main: receive from ch", func() {
		<-ch
	})
}
