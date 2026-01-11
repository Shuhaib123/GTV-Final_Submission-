package main

import "context"

const stop = -1

func producer(ctx context.Context, out chan<- int) {
	for i := 0; i < 3; i++ {
		// gtv:send=stage1
		out <- i
	}
	// gtv:send=stage1
	out <- stop
	_ = ctx
}

func transformer(ctx context.Context, in <-chan int, out chan<- int) {
	var v int
	for {
		// gtv:recv=stage1
		v = <-in
		if v == stop {
			// gtv:send=stage2
			out <- stop
			break
		}
		// gtv:send=stage2
		out <- v * 2
	}
	_ = ctx
}

func aggregator(ctx context.Context, in <-chan int, done chan<- int) {
	sum := 0
	var v int
	for {
		// gtv:recv=stage2
		v = <-in
		if v == stop {
			break
		}
		sum += v
	}
	done <- sum
	_ = ctx
}

func main() {
	stage1 := make(chan int)
	stage2 := make(chan int)
	done := make(chan int, 1)

	go producer(stage1)
	go transformer(stage1, stage2)
	go aggregator(stage2, done)

	<-done
}
