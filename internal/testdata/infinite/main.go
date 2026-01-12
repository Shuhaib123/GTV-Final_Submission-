package main

import (
	"sync"
)

func main() {
	ch := make(chan int, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case ch <- 1:
			default:
			}
		}
	}()
	select {}
}
