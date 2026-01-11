package main

import (
	"context"
	"sync"
)

const (
	workers = 2
	total   = 4
	stop    = -1
)

func producer(ctx context.Context, out chan<- int) {
	for i := 0; i < total; i++ {
		// gtv:send=in
		out <- i
	}
	for i := 0; i < workers; i++ {
		// gtv:send=in
		out <- stop
	}
	_ = ctx
}

func worker(ctx context.Context, in <-chan int, out chan<- int, wg *sync.WaitGroup) {
	defer wg.Done()
	var v int
	for {
		// gtv:recv=in
		v = <-in
		if v == stop {
			return
		}
		// gtv:send=out
		out <- v * v
	}
}

func closer(ctx context.Context, wg *sync.WaitGroup, out chan<- int) {
	wg.Wait()
	// gtv:send=out
	out <- stop
	close(out)
	_ = ctx
}

func main() {
	in := make(chan int, total+workers)
	out := make(chan int, total+1)

	go producer(in)

	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go worker(in, out, &wg)
	}

	go closer(&wg, out)

	sum := 0
	var v int
	for {
		// gtv:recv=out
		v = <-out
		if v == stop {
			break
		}
		sum += v
	}
	_ = sum
}
