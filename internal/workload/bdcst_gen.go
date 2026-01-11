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

var __jspt_spawn_id_2 uint64

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

func (s *Server) register(__jspt_ctx_7 context.Context, r *Request) {
	s.clientOut[s.num] = r.out
	s.num++
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_7,
			"ch_ptr",

			fmt.Sprintf("ptr=%p", r.
				reg))
	}
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_7,
			"ch_ptr",

			fmt.Sprintf("ptr=%p", r.reg,
			))
	}
	trace.WithRegion(__jspt_ctx_7, "worker: send to reg", func() {
		{

			// gtv:send=reg
			r.reg <- s.clientIn
		}
	})
}

func (s *Server) broadcast(__jspt_ctx_8 context.Context, msg string) {
	for i := 0; i < s.num; i++ {
		if trace.IsEnabled() {
			trace.Log(__jspt_ctx_8,
				"ch_ptr",

				fmt.Sprintf("ptr=%p", s.
					clientOut[i]))
		}
		if trace.IsEnabled() {
			if trace.IsEnabled() {
				trace.Log(__jspt_ctx_8,
					"ch_ptr",

					fmt.Sprintf("ptr=%p", s.clientOut[i]))
			}
			trace.WithRegion(__jspt_ctx_8, fmt.Sprintf("worker: send to clientout[%v]", i), func() {
				{

					// gtv:send=clientout[i]
					s.clientOut[i] <- msg
				}
			})
		} else

		// gtv:role=server
		{
			{
				s.clientOut[i] <- msg
			}
		}
	}
}

func (s *Server) runServer(__jspt_ctx_9 context.Context,) {
	var r *Request
	var msg string

	fmt.Println("Server running...")
	s.num = 0
	s.clientIn = make(chan string, MAX)
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_9,
			"chan_make",

			fmt.Sprintf("ptr=%p cap=%d type=%s",
				s.clientIn, MAX,

				"string"))
	}

	for {
		{
			__jspt_sel_14 := fmt.Sprintf("s%v",
				time.Now().UnixNano())
			if trace.IsEnabled() {
				trace.Log(__jspt_ctx_9,
					"select",

					"select_begin id="+__jspt_sel_14,
				)
			}

			select {
			case __jspt_recv_15 := <-__jspt_select_recv_3(
			// gtv:recv=join
			__jspt_ctx_9, "server: receive from "+

			// gtv:recv=clientin
			"join", s.join):
				if trace.IsEnabled() {
					trace.Log(__jspt_ctx_9,
						"select",

						"select_chosen id="+__jspt_sel_14,
					)
				}

				r = __jspt_recv_15

				s.register(__jspt_ctx_9, r)
				if trace.IsEnabled() {
					trace.Log(__jspt_ctx_9,
						"value",

						fmt.Sprint(__jspt_recv_15),
					)
				}

			case __jspt_recv_16 := <-__jspt_select_recv_3(__jspt_ctx_9, "server: receive from "+

			// client ID
			"clientin", s.clientIn):
				if trace.IsEnabled() {
					trace.Log(__jspt_ctx_9,
						"select",

						"select_chosen id="+__jspt_sel_14,
					)
				}

				msg = __jspt_recv_16
				s.broadcast(__jspt_ctx_9, msg)
				if trace.IsEnabled() {
					trace.Log(__jspt_ctx_9,
						"value",

						fmt.Sprint(__jspt_recv_16),
					)
				}

			}
			if trace.IsEnabled() {
				trace.Log(__jspt_ctx_9,
					"select",

					"select_end id="+__jspt_sel_14,
				)
			}
		}

	}
}

type Client struct {
	num	int
	// channel for receipt of serverOut from server
	join	chan chan<- string
	// channel for msg from server
	serverIn	chan string
	// channel for msg to server
	serverOut	chan<- string
}

func (c *Client) register(__jspt_ctx_10 context.Context, server chan *Request) {
	c.serverIn = make(chan string, 1)
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_10,
			"chan_make",

			fmt.Sprintf("ptr=%p cap=%d type=%s",
				c.serverIn, 1,
				"string",
			))
	}

	c.join = make(chan chan<- string, 1)
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_10,
			"chan_make",

			fmt.Sprintf("ptr=%p cap=%d type=%s",
				c.join, 1, "chan<- string",
			))
	}

	request := &Request{c.join, c.serverIn}
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_10,
			"ch_ptr",

			fmt.Sprintf("ptr=%p", server,
			))
	}
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_10,
			"ch_ptr",

			fmt.Sprintf("ptr=%p", server,
			))
	}
	trace.WithRegion(__jspt_ctx_10, "worker: send to join", func() {
		{

			// gtv:send=join
			server <- request
		}
	})
}

func (c *Client) broadcast(__jspt_ctx_11 context.
// gtv:send=clientin
Context, msg string) {
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_11,
			"ch_ptr",

			fmt.Sprintf("ptr=%p", c.
				serverOut))
	}
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_11,
			"ch_ptr",

			fmt.Sprintf("ptr=%p", c.
				serverOut))
	}
	trace.WithRegion(__jspt_ctx_11, "worker: send to clientin", func() {
		{

			c.serverOut <- msg
		}
	})
}

func (c *Client) output(__jspt_ctx_12 context.Context, msg string) {
	fmt.Println("Client", c.num, ":", msg)
}

// gtv:role=client
func (c *Client) runClient(__jspt_ctx_13 context.Context, i int, server chan *Request) {
	var msg string

	fmt.Println("Client", i, "running...")
	c.num = i
	c.register(__jspt_ctx_13, server)
	sent := false
	for {
		select {
		// gtv:recv=reg
		case __jspt_recv_17 := <-__jspt_select_recv_3(
		// gtv:recv=clientout[i]
		__jspt_ctx_13, "client: receive from "+"join", c.join):
			c.serverOut = __jspt_recv_17
			if trace.IsEnabled() {
				trace.Log(__jspt_ctx_13,
					"value",

					fmt.Sprint(__jspt_recv_17,
					))
			}

		case __jspt_recv_18 := <-__jspt_select_recv_3(__jspt_ctx_13,

		// received = true
		"client: receive from "+"serverin", c.serverIn):
			msg = __jspt_recv_18
			c.output(__jspt_ctx_13, msg)
			if trace.IsEnabled() {
				trace.Log(__jspt_ctx_13,
					"value",

					fmt.Sprint(__jspt_recv_18,
					))
			}

		default:
			if c.num == 1 && c.serverOut != nil && !sent {
				c.broadcast(__jspt_ctx_13, "Hello World")
				sent = true
			}
		}
	}
}

func RunbdcstProgram(__jspt_ctx_5 context.Context) {
	var __jspt_wg_0 sync.WaitGroup
	defer __jspt_wg_0.Wait()
	__jspt_ctx_5, __jspt_task_6 := trace.NewTask(__jspt_ctx_5, "bdcst")
	defer __jspt_task_6.End()
	trace.Log(__jspt_ctx_5, "main", "bdcst starting")
	s := new(Server)
	s.join = make(chan *Request, MAX)
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_5,
			"chan_make",

			fmt.Sprintf("ptr=%p cap=%d type=%s",
				s.join, MAX, "*Request",
			))
	}

	fmt.Println("Starting up clients...")
	for i := 0; i < MAX; i++ {
		c := new(Client)
		__jspt_spawn_19 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
		trace.Log(__jspt_ctx_5,

			"spawn_parent",

			fmt.Sprintf("sid=%d",
				__jspt_spawn_19,
			))
		__jspt_wg_0.Add(1)
		go func(__jspt_ctx_20 context.Context) {
			trace.WithRegion(__jspt_ctx_20, "goroutine: anon", func() {
				{
					defer __jspt_wg_0.Done()
					trace.Log(__jspt_ctx_5,

						"spawn_child",

						fmt.Sprintf("sid=%d",
							__jspt_spawn_19,
						))

					c.runClient(__jspt_ctx_5, i, s.join)
				}
			})
		}(__jspt_ctx_5)
	}
	s.runServer(__jspt_ctx_5)
}
func __jspt_select_recv_3[T any](__jspt_ctx_21 context.Context, label string, ch <-chan T) <-chan T {
	if trace.
		IsEnabled() {
		trace.Log(__jspt_ctx_21, "ch_ptr",
			fmt.Sprintf("ptr=%p",

				ch))
		trace.Log(__jspt_ctx_21,

			"ch_name", fmt.Sprintf("ptr=%p name=%s", ch,
				label))
	}
	trace.WithRegion(__jspt_ctx_21, label, func() {})
	return ch
}
func __jspt_select_send_4[T any](__jspt_ctx_21 context.
	Context, label string, ch chan<- T) chan<- T {
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_21,
			"ch_ptr",
			fmt.Sprintf("ptr=%p",
				ch))
		trace.
			Log(__jspt_ctx_21, "ch_name", fmt.Sprintf("ptr=%p name=%s",
				ch, label))
	}
	trace.WithRegion(__jspt_ctx_21,
		label, func() {})
	return ch
}
func init() {
	RegisterWorkload("bdcst",
		RunbdcstProgram,
	)
}
