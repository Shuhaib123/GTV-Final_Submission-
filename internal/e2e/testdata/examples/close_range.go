package main

import "context"

func sender(ctx context.Context, ch chan<- int, done chan<- struct{}) {
	for i := 0; i < 3; i++ {
		// gtv:send=ch
		ch <- i
	}
	close(ch)
	// gtv:send=done
	done <- struct{}{}
	_ = ctx
}

func main() {
	ch := make(chan int)
	done := make(chan struct{}, 1)

	go sender(ch, done)

	for range ch {
	}
	// gtv:recv=done
	<-done
}
