//go:build workload_tpc
// +build workload_tpc

package workload

import (
	"context"
	"fmt"

	// at most 10 clients
	"jspt/internal/gtvtrace"
	"runtime/trace"

	// send-only channel: chan<-
	// receive-only channel: <-chan
	"sync"
	"sync/atomic"
	"time"
)

var __jspt_spawn_id_2 uint64

const MAX = 10

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

func (s *Server) register(__jspt_ctx_10 context.Context, r *Request) {
	s.clientOut[s.num] = r.out
	s.num++
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_10,
			"value",

			fmt.Sprint(s.clientIn))
	}
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_10,
			"ch_ptr",

			fmt.Sprintf("ptr=%p", r.
				reg))
	}
	trace.WithRegion(__jspt_ctx_10, "worker: send to reg", func() {
		{

			// gtv:send=reg
			r.reg <- s.clientIn
		}
	})
}

func (s *Server) broadcast(__jspt_ctx_11 context.Context, msg string) {
	for i := 0; i < s.num; i++ {
		if trace.IsEnabled() {
			trace.Log(__jspt_ctx_11,
				"value",

				fmt.Sprint(msg))
		}
		if trace.IsEnabled() {
			trace.Log(__jspt_ctx_11,
				"ch_ptr",

				fmt.Sprintf("ptr=%p", s.
					clientOut[i]))
		}
		if trace.IsEnabled() {
			if trace.IsEnabled() {
				trace.Log(__jspt_ctx_11,
					"ch_ptr",

					fmt.Sprintf("ptr=%p", s.
						clientOut[i]))
			}
			trace.WithRegion(__jspt_ctx_11, fmt.Sprintf("worker: send to clientout[%v]", i), func() {
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

func (s *Server) runServer(__jspt_ctx_12 context.Context) {
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_12, "server", fmt.Sprintf("server running"))
	}
	trace.Log(__jspt_ctx_12, "role", "server")
	var r *Request
	trace.Log(__jspt_ctx_12, "role", "server")
	var msg string
	trace.Log(__jspt_ctx_12, "role", "server")

	fmt.Println("Server running...")
	trace.Log(__jspt_ctx_12, "role", "server")
	s.num = 0
	trace.Log(__jspt_ctx_12, "role", "server")
	s.clientIn = make(chan string, MAX)
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_12,
			"chan_make",

			fmt.Sprintf("ptr=%p cap=%d type=%s",
				s.clientIn, MAX,

				"string"))
	}
	trace.Log(__jspt_ctx_12, "role", "server")

	for {
		trace.Log(__jspt_ctx_12, "role", "server")

		// gtv:recv=join
		{
			__jspt_sel_17 := fmt.Sprintf("s%v",
				time.Now().UnixNano())
			if trace.IsEnabled() {
				trace.Log(__jspt_ctx_12,
					"select",

					"select_begin id="+__jspt_sel_17,
				)
			}

			select {
			case __jspt_recv_18 := <-__jspt_recv_in_region_4(__jspt_ctx_12,

				// gtv:recv=clientin
				"server: receive from "+"join", s.join):
				if trace.IsEnabled() {
					trace.Log(__jspt_ctx_12,
						"select",

						"select_chosen id="+__jspt_sel_17,
					)
				}
				if trace.IsEnabled() {
					trace.Log(__jspt_ctx_12,
						"select_recv",

						fmt.Sprintf("ptr=%p name=%s",
							s.join, "server: receive from "+
								"join"))
				}
				trace.Log(__jspt_ctx_12, "role", "server")

				r = __jspt_recv_18

				s.register(__jspt_ctx_12, r)
				if trace.IsEnabled() {
					trace.Log(__jspt_ctx_12,
						"value",

						fmt.Sprint(__jspt_recv_18))
				}

			case __jspt_recv_19 := <-__jspt_recv_in_region_4(__jspt_ctx_12, "server: receive from "+

				// client ID
				"clientin", s.clientIn):
				if trace.IsEnabled() {
					trace.Log(__jspt_ctx_12,
						"select",

						"select_chosen id="+__jspt_sel_17,
					)
				}
				if trace.IsEnabled() {
					trace.Log(__jspt_ctx_12,
						"select_recv",

						fmt.Sprintf("ptr=%p name=%s",
							s.clientIn, "server: receive from "+
								"clientin",
						))
				}

				msg = __jspt_recv_19
				s.broadcast(__jspt_ctx_12, msg)
				if trace.IsEnabled() {
					trace.Log(__jspt_ctx_12,
						"value",

						fmt.Sprint(__jspt_recv_19))
				}

			}
			if trace.IsEnabled() {
				trace.Log(__jspt_ctx_12,
					"select",

					"select_end id="+__jspt_sel_17,
				)
			}
		}

	}
}

type Client struct {
	num int
	// channel for receipt of serverOut from server
	join chan chan<- string
	// channel for msg from server
	serverIn chan string
	// channel for msg to server
	serverOut chan<- string
}

func (c *Client) register(__jspt_ctx_13 context.Context, server chan *Request) {
	c.serverIn = make(chan string, 1)
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_13,
			"chan_make",

			fmt.Sprintf("ptr=%p cap=%d type=%s",
				c.serverIn, 1,
				"string",
			))
	}

	c.join = make(chan chan<- string, 1)
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_13,
			"chan_make",

			fmt.Sprintf("ptr=%p cap=%d type=%s",
				c.join, 1, "chan<- string",
			))
	}

	request := &Request{c.join, c.serverIn}
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_13,
			"value",

			fmt.Sprint(request))
	}
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_13,
			"ch_ptr",

			fmt.Sprintf("ptr=%p", server))
	}
	trace.WithRegion(__jspt_ctx_13, "worker: send to join", func() {
		{

			// gtv:send=join
			server <- request
		}
	})
}

func (c *Client) broadcast(__jspt_ctx_14 context.
	// gtv:send=clientin
	Context, msg string) {
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_14,
			"value",

			fmt.Sprint(msg))
	}
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_14,
			"ch_ptr",

			fmt.Sprintf("ptr=%p", c.
				serverOut))
	}
	trace.WithRegion(__jspt_ctx_14, "worker: send to clientin", func() {
		{

			c.serverOut <- msg
		}
	})
}

func (c *Client) output(__jspt_ctx_15 context.Context, msg string) {
	fmt.Println("Client", c.num, ":", msg)
}

// gtv:role=client
func (c *Client) runClient(__jspt_ctx_16 context.Context, i int, server chan *Request) {
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_16, "client", fmt.Sprintf("client running"))
	}
	trace.Log(__jspt_ctx_16, "role", "client")
	var msg string
	trace.Log(__jspt_ctx_16, "role", "client")

	fmt.Println("Client", i, "running...")
	trace.Log(__jspt_ctx_16, "role", "client")
	c.num = i
	trace.Log(__jspt_ctx_16, "role", "client")
	c.register(__jspt_ctx_16, server)
	trace.Log(__jspt_ctx_16, "role", "client")
	sent := false
	trace.Log(__jspt_ctx_16,

		// gtv:recv=reg
		"role", "client")
	for {
		trace.Log(__jspt_ctx_16, "role", "client")
		select {

		case __jspt_recv_20 := <-__jspt_recv_in_region_4(
			// gtv:recv=clientout[i]
			__jspt_ctx_16, "client: receive from "+"join", c.join):
			if trace.IsEnabled() {
				trace.Log(__jspt_ctx_16,
					"select_recv",

					fmt.Sprintf("ptr=%p name=%s",
						c.join, "client: receive from "+
							"join"))
			}

			c.serverOut = __jspt_recv_20
			if trace.IsEnabled() {
				trace.Log(__jspt_ctx_16,
					"value",

					fmt.Sprint(__jspt_recv_20))
			}

		case __jspt_recv_21 := <-__jspt_recv_in_region_4(

			// received = true
			__jspt_ctx_16, "client: receive from "+"serverin", c.serverIn):
			if trace.IsEnabled() {
				trace.Log(__jspt_ctx_16,
					"select_recv",

					fmt.Sprintf("ptr=%p name=%s",
						c.serverIn, "client: receive from "+
							"serverin",
					))
			}

			msg = __jspt_recv_21
			c.output(__jspt_ctx_16, msg)
			if trace.IsEnabled() {
				trace.Log(__jspt_ctx_16,
					"value",

					fmt.Sprint(__jspt_recv_21))
			}

		default:
			if c.num == 1 && c.serverOut != nil && !sent {
				c.broadcast(__jspt_ctx_16, "Hello World")
				sent = true
			}
			time.
				Sleep(1 *
					time.Millisecond)

		}
	}
}

func RuntpcProgram(__jspt_ctx_8 context.Context) {
	var __jspt_wg_0 sync.WaitGroup
	defer __jspt_wg_0.Wait()
	gtvtrace.InstallStopOnSignal()
	gtvtrace.InstallStopAfterFromEnv(
		"GTV_TIMEOUT_MS")
	__jspt_ctx_8, __jspt_task_9 := trace.NewTask(__jspt_ctx_8, "tpc")
	defer __jspt_task_9.End()
	trace.Log(__jspt_ctx_8, "main", "tpc starting")

	s := new(Server)
	s.join = make(chan *Request, MAX)
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_8,
			"chan_make",

			fmt.Sprintf("ptr=%p cap=%d type=%s",
				s.join, MAX, "*Request",
			))
	}

	fmt.Println("Starting up clients...")
	for i := 0; i < MAX; i++ {
		c := new(Client)
		__jspt_spawn_22 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
		trace.Log(__jspt_ctx_8,

			"spawn_parent",

			fmt.Sprintf("sid=%d",
				__jspt_spawn_22,
			))
		__jspt_wg_0.Add(1)
		go func(__jspt_ctx_23 context.Context) {
			trace.WithRegion(__jspt_ctx_23, "goroutine: anon", func() {
				{
					defer __jspt_wg_0.Done()
					trace.Log(__jspt_ctx_8,

						"spawn_child",

						fmt.Sprintf("sid=%d",
							__jspt_spawn_22,
						))

					c.runClient(__jspt_ctx_8, i, s.join)
				}
			})
		}(__jspt_ctx_8)
	}
	s.runServer(__jspt_ctx_8)
}
func __jspt_select_recv_3[T any](__jspt_ctx_24 context.Context, label string, ch <-chan T) <-chan T {
	if trace.
		IsEnabled() {
		trace.Log(__jspt_ctx_24, "ch_ptr",
			fmt.Sprintf("ptr=%p",

				ch))
		trace.Log(__jspt_ctx_24,

			"ch_name", fmt.Sprintf("ptr=%p name=%s", ch,
				label))
	}
	return ch
}
func __jspt_recv_in_region_4[T any](__jspt_ctx_24 context.Context, label string, ch <-chan T) <-chan T {
	if trace.IsEnabled() {
		trace.
			Log(__jspt_ctx_24, "ch_ptr", fmt.Sprintf("ptr=%p", ch))
		trace.Log(__jspt_ctx_24,
			"ch_name",
			fmt.Sprintf("ptr=%p name=%s",

				ch, label))
	}
	return ch
}
func __jspt_select_send_5[T any](
	__jspt_ctx_24 context.Context, label string, ch chan<- T) chan<- T {
	if trace.
		IsEnabled() {
		trace.Log(__jspt_ctx_24,
			"ch_ptr",
			fmt.Sprintf("ptr=%p", ch))
		trace.Log(__jspt_ctx_24,
			"ch_name",
			fmt.Sprintf("ptr=%p name=%s",
				ch,
				label))
	}
	return ch
}
func init() {
	RegisterWorkload("tpc",
		RuntpcProgram,
	)
}
