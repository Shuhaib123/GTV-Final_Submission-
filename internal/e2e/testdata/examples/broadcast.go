package main

import "context"

func client(ctx context.Context, inbox <-chan string, done chan<- struct{}) {
	// gtv:recv=inbox
	<-inbox
	done <- struct{}{}
	_ = ctx
}

func main() {
	clientA := make(chan string)
	clientB := make(chan string)
	done := make(chan struct{}, 2)

	go client(clientA, done)
	go client(clientB, done)

	msg := "hello"
	// gtv:send=clientA
	clientA <- msg
	// gtv:send=clientB
	clientB <- msg

	<-done
	<-done
}
