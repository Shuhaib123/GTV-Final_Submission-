package main

import (
	"fmt"
	"sync"
)

func worker(id int, jobs <-chan int, results chan<- int, wg *sync.WaitGroup) {
	defer wg.Done()
	for job := range jobs {
		fmt.Printf("worker %d processing %d\n", id, job)
		results <- job * job
	}
}

func main() {
	const pool = 3
	jobs := make(chan int)
	results := make(chan int)
	var poolWG sync.WaitGroup
	for w := 1; w <= pool; w++ {
		poolWG.Add(1)
		go worker(w, jobs, results, &poolWG)
	}
	go func() {
		for i := 1; i <= 5; i++ {
			jobs <- i
		}
		close(jobs)
	}()
	go func() {
		poolWG.Wait()
		close(results)
	}()
	for res := range results {
		fmt.Println("result", res)
	}
}
