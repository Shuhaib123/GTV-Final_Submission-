package main

import (
	"fmt"
)

// at most 10 clients
const MAX = 10

// send-only channel: chan<-
// receive-only channel: <-chan

type Request struct {
	// for reply with clientIn
	reg chan chan<- string
	// for clientOut channel
	out chan<- string
}

type Server struct {
	num int
	// join channel known to client when it joins
	join chan *Request
	// channel for incoming msgs from clients
	clientIn chan string
	// channels for outgoing msgs to clients
	clientOut [MAX]chan<- string
}

func (s *Server) register(r *Request) {
	s.clientOut[s.num] = r.out
	s.num++
	r.reg <- s.clientIn
}

func (s *Server) broadcast(msg string) {
	for i := 0; i < s.num; i++ {
		s.clientOut[i] <- msg
	}
}

func (s *Server) runServer() {
	var r *Request
	var msg string

	fmt.Println("Server running...")
	s.num = 0
	s.clientIn = make(chan string, MAX)
	for {
		select {
		case r = <-s.join:
			s.register(r)
		// gtv:recv=clientin
		case msg = <-s.clientIn:
			s.broadcast(msg)
		}
	}
}

type Client struct {
	// client ID
	num int
	// channel for receipt of serverOut from server
	join chan chan<- string
	// channel for msg from server
	serverIn chan string
	// channel for msg to server
	serverOut chan<- string
}

func (c *Client) register(server chan *Request) {
	c.serverIn = make(chan string, 1)
	c.join = make(chan chan<- string, 1)
	request := &Request{c.join, c.serverIn}
	server <- request
}

func (c *Client) broadcast(msg string) {
	c.serverOut <- msg
}

func (c *Client) output(msg string) {
	fmt.Println("Client", c.num, ":", msg)
}

func (c *Client) runClient(i int, server chan *Request) {
	var msg string

	fmt.Println("Client", i, "running...")
	c.num = i
	c.register(server)
	sent := false
	for {
		select {
		case c.serverOut = <-c.join:
		// gtv:recv=serverout
		case msg = <-c.serverIn:
			c.output(msg)
			// received = true
		default:
			if c.num == 1 && c.serverOut != nil && !sent {
				c.broadcast("Hello World")
				sent = true
			}
		}
	}
}

func main() {
	s := new(Server)
	s.join = make(chan *Request, MAX)
	fmt.Println("Starting up clients...")
	for i := 0; i < MAX; i++ {
		c := new(Client)
		go c.runClient(i, s.join)
	}
	s.runServer()
}
