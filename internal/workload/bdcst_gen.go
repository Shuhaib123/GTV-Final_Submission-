//go:build workload_bdcst
// +build workload_bdcst

package workload

import (
	"fmt"
	"context"

	// at most 10 clients
	"runtime/trace"
	"time"
	"sync/atomic"

	// send-only channel: chan<-
	// receive-only channel: <-chan
	"sync"
)

var gtvSpawnID uint64

const MAX = 10

type Request struct {
	// for reply with clientIn
	reg	chan chan<- string
	// for clientOut channel
	out	chan<- string
}

type Server struct {
	num	int
	// join channel known to client when it joins
	join	chan *Request
	// channel for incoming msgs from clients
	clientIn	chan string
	// channels for outgoing msgs to clients
	clientOut	[MAX]chan<- string
}

func (s *Server) register(ctx context.Context, r *Request) {
	s.clientOut[s.num] = r.out
	s.num++
	if trace.IsEnabled() {
		trace.Log(ctx,
			"value", fmt.
				Sprint(s.clientIn))
	}
	if trace.IsEnabled() {
		trace.Log(ctx,
			"ch_ptr", fmt.
				Sprintf("ptr=%p", r.reg))
	}
	if trace.IsEnabled() {
		trace.Log(ctx,
			"ch_ptr", fmt.
				Sprintf("ptr=%p", r.reg))
	}
	trace.WithRegion(ctx, "worker: send to "+"reg", func() {
		{

			r.reg <- s.clientIn
		}
	})
}

func (s *Server) broadcast(ctx context.Context, msg string) {
	for i := 0; i < s.num; i++ {
		if trace.IsEnabled() {
			trace.Log(ctx,
				"value", fmt.
					Sprint(msg))
		}
		if trace.IsEnabled() {
			trace.Log(ctx,
				"ch_ptr", fmt.
					Sprintf("ptr=%p", s.clientOut[i]))
		}
		if trace.IsEnabled() {
			if trace.IsEnabled() {
				trace.Log(ctx,
					"ch_ptr", fmt.
						Sprintf("ptr=%p", s.clientOut[i]))
			}
			trace.WithRegion(ctx, "worker: send to "+fmt.Sprintf("clientout[%v]", i), func() {
				{

					s.clientOut[i] <- msg
				}
			})
		} else {
			{
				s.clientOut[i] <- msg
			}
		}
	}
}

func (s *Server) runServer(ctx context.Context,) {
	var r *Request
	var msg string

	fmt.Println("Server running...")
	s.num = 0
	s.clientIn = make(chan string, MAX)
	if trace.IsEnabled() {
		trace.Log(ctx,
			"chan_make",
			fmt.
				Sprintf("ptr=%p cap=%d type=%s",
					s.clientIn, MAX, "string",
				),
		)
	}

	for {
		{
			__gtvSel1 :=

				fmt.Sprintf("s%v", time.
					Now().UnixNano())
			if trace.IsEnabled() {
				trace.Log(ctx,
					"select", "select_begin id="+
						__gtvSel1)
			}

			select {
			case __gtvRecv1 := <-s.join:
				if trace.IsEnabled() {
					trace.Log(ctx,
						"select", "select_chosen id="+
							__gtvSel1)
				}
				if trace.IsEnabled() {
					trace.WithRegion(ctx, "worker: receive from "+"join", func() {
						{
							if trace.IsEnabled() {
								trace.Log(ctx,
									"ch_ptr", fmt.
										Sprintf("ptr=%p", s.join))
							}

							r = __gtvRecv1
							s.register(ctx, r)
							if trace.IsEnabled() {
								trace.Log(ctx,
									"value", fmt.Sprint(__gtvRecv1))
							}
						}
					})
				} else {
					{
						if trace.IsEnabled() {
							trace.Log(ctx,
								"ch_ptr", fmt.
									Sprintf("ptr=%p", s.join))
						}

						r = __gtvRecv1
						s.register(ctx, r)
						if trace.IsEnabled() {
							trace.Log(ctx,
								"value", fmt.Sprint(__gtvRecv1))
						}
					}
				}

			case __gtvRecv2 := <-s.clientIn:
				if trace.IsEnabled() {
					trace.Log(ctx,
						"select", "select_chosen id="+
							__gtvSel1)
				}
				if trace.IsEnabled() {
					trace.WithRegion(ctx, "worker: receive from "+"clientin", func() {
						{
							if trace.IsEnabled() {
								trace.Log(ctx,
									"ch_ptr", fmt.
										Sprintf("ptr=%p", s.clientIn))
							}

							msg = __gtvRecv2
							s.broadcast(ctx, msg)
							if trace.IsEnabled() {
								trace.Log(ctx,
									"value", fmt.Sprint(__gtvRecv2))
							}
						}
					})
				} else {
					{
						if trace.IsEnabled() {
							trace.Log(ctx,
								"ch_ptr", fmt.
									Sprintf("ptr=%p", s.clientIn))
						}

						msg = __gtvRecv2
						s.broadcast(ctx, msg)
						if trace.IsEnabled() {
							trace.Log(ctx,
								"value", fmt.Sprint(__gtvRecv2))
						}
					}
				}

			}
			if trace.IsEnabled() {
				trace.Log(ctx,
					"select", "select_end id="+
						__gtvSel1)
			}
		}

	}
}

type Client struct {
	// client ID
	num	int
	// channel for receipt of serverOut from server
	join	chan chan<- string
	// channel for msg from server
	serverIn	chan string
	// channel for msg to server
	serverOut	chan<- string
}

func (c *Client) register(ctx context.Context, server chan *Request) {
	c.serverIn = make(chan string, 1)
	if trace.IsEnabled() {
		trace.Log(ctx,
			"chan_make",
			fmt.
				Sprintf("ptr=%p cap=%d type=%s",
					c.serverIn, 1, "string"))

	}

	c.join = make(chan chan<- string, 1)
	if trace.IsEnabled() {
		trace.Log(ctx,
			"chan_make",
			fmt.
				Sprintf("ptr=%p cap=%d type=%s",
					c.join, 1, "chan<- string",
				))
	}

	request := &Request{c.join, c.serverIn}
	if trace.IsEnabled() {
		trace.Log(ctx,
			"value", fmt.
				Sprint(request))
	}
	if trace.IsEnabled() {
		trace.Log(ctx,
			"ch_ptr", fmt.
				Sprintf("ptr=%p", server))
	}
	if trace.IsEnabled() {
		trace.Log(ctx,
			"ch_ptr", fmt.
				Sprintf("ptr=%p", server))
	}
	trace.WithRegion(ctx, "worker: send to "+"server", func() {
		{

			server <- request
		}
	})
}

func (c *Client) broadcast(ctx context.Context, msg string) {
	if trace.IsEnabled() {
		trace.Log(ctx,
			"value", fmt.
				Sprint(msg))
	}
	if trace.IsEnabled() {
		trace.Log(ctx,
			"ch_ptr", fmt.
				Sprintf("ptr=%p", c.serverOut,
				))
	}
	if trace.IsEnabled() {
		trace.Log(ctx,
			"ch_ptr", fmt.
				Sprintf("ptr=%p", c.serverOut),
		)
	}
	trace.WithRegion(ctx, "worker: send to "+"serverout", func() {
		{

			c.serverOut <- msg
		}
	})
}

func (c *Client) output(ctx context.Context, msg string) {
	fmt.Println("Client", c.num, ":", msg)
}

func (c *Client) runClient(ctx context.Context, i int, server chan *Request) {
	var msg string

	fmt.Println("Client", i, "running...")
	c.num = i
	c.register(ctx, server)
	sent := false
	for {
		select {
		case __gtvRecv3 := <-c.join:
			if trace.IsEnabled() {
				trace.WithRegion(ctx, "worker: receive from "+

				// received = true
				"join", func() {
					{
						if trace.IsEnabled() {
							trace.Log(ctx,
								"ch_ptr", fmt.
									Sprintf("ptr=%p", c.join))
						}

						c.serverOut = __gtvRecv3
						if trace.IsEnabled() {
							trace.Log(ctx,
								"value", fmt.Sprint(__gtvRecv3))
						}
					}
				})
			} else {
				{
					if trace.IsEnabled() {
						trace.Log(ctx,
							"ch_ptr", fmt.
								Sprintf("ptr=%p", c.join))
					}

					c.serverOut = __gtvRecv3
					if trace.IsEnabled() {
						trace.Log(ctx,
							"value", fmt.Sprint(__gtvRecv3))
					}
				}
			}

		case __gtvRecv4 := <-c.serverIn:
			if trace.IsEnabled() {
				trace.WithRegion(ctx, "worker: receive from "+"serverin", func() {
					{
						if trace.IsEnabled() {
							trace.Log(ctx,
								"ch_ptr", fmt.
									Sprintf("ptr=%p", c.serverIn))
						}

						msg = __gtvRecv4
						c.output(ctx, msg)
						if trace.IsEnabled() {
							trace.Log(ctx,
								"value", fmt.Sprint(__gtvRecv4))
						}
					}
				})
			} else {
				{
					if trace.IsEnabled() {
						trace.Log(ctx,
							"ch_ptr", fmt.
								Sprintf("ptr=%p", c.serverIn))
					}

					msg = __gtvRecv4
					c.output(ctx, msg)
					if trace.IsEnabled() {
						trace.Log(ctx,
							"value", fmt.Sprint(__gtvRecv4))
					}
				}
			}

		default:
			if c.num == 1 && c.serverOut != nil && !sent {
				c.broadcast(ctx, "Hello World")
				sent = true
			}
		}
	}
}

func RunbdcstProgram(ctx context.Context) {
	var wg sync.WaitGroup
	defer wg.Wait()
	ctx, task := trace.NewTask(ctx, "bdcst")
	defer task.End()
	trace.Log(ctx, "main", "bdcst starting")
	s := new(Server)
	s.join = make(chan *Request, MAX)
	if trace.IsEnabled() {
		trace.Log(ctx,
			"chan_make",
			fmt.
				Sprintf("ptr=%p cap=%d type=%s",
					s.join, MAX, "*Request"))

	}

	fmt.Println("Starting up clients...")
	for i := 0; i < MAX; i++ {
		c := new(Client)
		__gtvSpawn1 := atomic.AddUint64(&gtvSpawnID, 1)
		trace.Log(ctx,

			"spawn_parent",

			fmt.
				Sprintf("sid=%d", __gtvSpawn1,
				))
		wg.Add(1)
		go func(ctx context.Context) {
			trace.WithRegion(ctx, "goroutine: anon", func() {
				{
					defer wg.Done()
					trace.Log(ctx,

						"spawn_child",

						fmt.
							Sprintf("sid=%d", __gtvSpawn1,
							))

					c.runClient(ctx, i, s.join)
				}
			})
		}(ctx)
	}
	s.runServer(ctx)
}
func init() {
	RegisterWorkload("bdcst",
		RunbdcstProgram,
	)
}
