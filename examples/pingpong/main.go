package main

import (
	"fmt"
	"sync"
)

func main() {
	ch := make(chan string)
	var workerWG sync.WaitGroup
	workerWG.Add(1)
	go func() {
		defer workerWG.Done()
		for i := 0; i < 3; i++ {
			msg := fmt.Sprintf("ping %d", i)
			ch <- msg
			fmt.Println("worker received", <-ch)
		}
	}()
	for i := 0; i < 3; i++ {
		fmt.Println("main received", <-ch)
		ch <- fmt.Sprintf("pong %d", i)
	}
	workerWG.Wait()
}
