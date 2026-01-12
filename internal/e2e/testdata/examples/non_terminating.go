package main

func main() {
	ch := make(chan int)
	go func() {
		// gtv:send=ping
		ch <- 1
		select {}
	}()
	// gtv:recv=ping
	<-ch
	select {}
}
