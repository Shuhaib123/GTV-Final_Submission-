package main

import (
	"context"
	"time"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	out := make(chan int)

	go func() {
		out <- 1
		<-ctx.Done()
	}()

	time.Sleep(2 * time.Millisecond)
	<-out
	<-ctx.Done()
}
