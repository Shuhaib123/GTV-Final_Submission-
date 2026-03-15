package main

import (
	"fmt"
)

func main() {
	ch := make(chan int)
	done := make(chan struct{})

	go func() {
		ch <- 42
		close(done)
	}()

	select {
	case v := <-ch:
		fmt.Println("got", v)
	}

	<-done
}
