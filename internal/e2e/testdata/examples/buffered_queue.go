package main

func main() {
	queue := make(chan int, 2)
	// gtv:send=queue
	queue <- 1
	// gtv:send=queue
	queue <- 2

	go func() {
		// gtv:send=queue
		queue <- 3
	}()

	for i := 0; i < 3; i++ {
		// gtv:recv=queue
		<-queue
	}
}
