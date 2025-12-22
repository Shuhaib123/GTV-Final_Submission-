package main

import (
	"fmt"
	"sync"
)

func main() {
	var chans []chan int
	for i := 0; i < 3; i++ {
		ch := make(chan int)
		chans = append(chans, ch)
		go func() {
			defer close(ch)
			ch <- i * 10
		}()
	}
	var dynamicWG sync.WaitGroup
	for _, ch := range chans {
		dynamicWG.Add(1)
		go func() {
			defer dynamicWG.Done()
			for v := range ch {
				fmt.Println("dynamic value", v)
			}
		}()
	}
	dynamicWG.Wait()
}
