package workload

import (
	"context"
	"fmt"
	"runtime/trace"
	"time"
)

// RunPingPongProgram runs the demo workload using the provided context.
// The caller is responsible for managing trace start/stop and task scope.
func RunPingPongProgram(ctx context.Context) {
	trace.Log(ctx, "main", "creating channels")
	ping := make(chan int)
	pong := make(chan int)

	trace.Log(ctx, "main", "starting worker goroutine")
	done := make(chan struct{})
	go PingPong(ctx, ping, pong, done)

	trace.WithRegion(ctx, "main: send 1 to ping", func() {
		ping <- 1
		trace.Log(ctx, "main", "sent 1 to ping")
	})

	// Intentionally delay before receiving so the worker's send to pong occurs
	// while main is not ready, forcing the worker to block on send.
	trace.WithRegion(ctx, "main: delay before receive", func() {
		time.Sleep(2 * time.Millisecond)
		trace.Log(ctx, "main", "delayed before receive")
	})

	trace.WithRegion(ctx, "main: receive from pong", func() {
		val := <-pong
		trace.Log(ctx, "main", fmt.Sprintf("received %d from pong", val))
	})

	// Ensure the worker finished its logging before returning.
	<-done
}

// PingPong is the worker goroutine body.
func PingPong(ctx context.Context, ping <-chan int, pong chan<- int, done chan<- struct{}) {
	region := trace.StartRegion(ctx, "worker: receive from ping")
	val := <-ping
	region.End()
	trace.Log(ctx, "worker", fmt.Sprintf("got %d from ping", val))

	region = trace.StartRegion(ctx, "worker: send to pong")
	pong <- val
	region.End()
	trace.Log(ctx, "worker", fmt.Sprintf("sent %d to pong", val))

	close(done)
}
