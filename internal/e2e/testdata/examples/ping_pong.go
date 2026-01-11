package main

import "context"

func worker(ctx context.Context, ping <-chan int, pong chan<- int, done chan<- struct{}) {
	var v int
	// gtv:recv=ping
	v = <-ping
	// gtv:send=pong
	pong <- v
	close(done)
	_ = ctx
}

func main() {
	ping := make(chan int, 1)
	pong := make(chan int, 1)
	done := make(chan struct{})

	go worker(ping, pong, done)

	// gtv:send=ping
	ping <- 1
	// gtv:recv=pong
	<-pong
	<-done
}
