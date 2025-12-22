package main

import (
	"fmt"
	"time"
)

func main() {
	ch := make(chan int)
	done := make(chan struct{})

	go func() {
		defer close(ch)
		for i := 0; i < 5; i++ {
			select {
			case ch <- i:
				fmt.Println("select send", i)
			case <-time.After(20 * time.Millisecond):
				fmt.Println("select send timeout", i)
			}
		}
	}()

	go func() {
		defer close(done)
		for {
			select {
			case v, ok := <-ch:
				if !ok {
					return
				}
				fmt.Println("select recv", v)
			case <-time.After(50 * time.Millisecond):
				fmt.Println("select recv timeout")
				return
			}
		}
	}()

	<-done
}
