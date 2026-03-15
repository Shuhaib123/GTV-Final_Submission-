package main

func main() {
	ch1 := make(chan int)
	ch2 := make(chan int)
	ready := make(chan struct{}, 2)
	readySend := make(chan struct{}, 2)
	startSend := make(chan struct{})

	go func() {
		ready <- struct{}{}
		<-ch2
		readySend <- struct{}{}
		<-startSend
		ch1 <- 1
	}()

	go func() {
		ready <- struct{}{}
		<-ch1
		readySend <- struct{}{}
		<-startSend
		ch2 <- 1
	}()

	<-ready
	<-ready
	ch1 <- 1
	ch2 <- 1
	<-readySend
	<-readySend
	close(startSend)

	select {}
}
