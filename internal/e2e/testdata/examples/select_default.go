package main

import "context"

func receiver(ctx context.Context, ch <-chan int, done chan<- struct{}) {
	// gtv:recv=ch
	<-ch
	close(done)
	_ = ctx
}

func main() {
	sel := make(chan int)
	ch := make(chan int, 1)
	done := make(chan struct{})

	select {
	case sel <- 1:
	default:
	}

	go receiver(ch, done)

	// gtv:send=ch
	ch <- 2
	<-done
}
