package main

import "fmt"

func main() {
	stage1 := make(chan int)
	stage2 := make(chan int)
	done := make(chan struct{})

	go func() {
		defer close(stage1)
		for i := 1; i <= 5; i++ {
			stage1 <- i
		}
	}()

	go func() {
		defer close(stage2)
		for v := range stage1 {
			stage2 <- v * 2
		}
	}()

	go func() {
		defer close(done)
		for v := range stage2 {
			fmt.Println("pipeline output", v)
		}
	}()

	<-done
}
