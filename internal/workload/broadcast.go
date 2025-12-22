//go:build !workload_broadcast
// +build !workload_broadcast

package workload

import (
	"context"
	"fmt"
	"os"
	"runtime/trace"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Broadcast demo modeled after the provided broadcast.go, but bounded for tracing.

const bcMax = 10

type runLimits struct {
	cancel   context.CancelFunc
	maxSteps int
	mu       sync.Mutex
	steps    int
}

func (rl *runLimits) step() {
	if rl == nil || rl.maxSteps <= 0 {
		return
	}
	rl.mu.Lock()
	rl.steps++
	done := rl.steps >= rl.maxSteps
	rl.mu.Unlock()
	if done {
		rl.cancel()
	}
}

func withBroadcastLimits(parent context.Context) (context.Context, context.CancelFunc, *runLimits) {
	dur := parseDurationEnv("GTV_MAX_MS")
	steps := parseIntEnv("GTV_MAX_STEPS")
	ctx, cancel := context.WithCancel(parent)
	var limits *runLimits
	if dur > 0 || steps > 0 {
		limits = &runLimits{cancel: cancel, maxSteps: steps}
		if dur > 0 {
			go func() {
				select {
				case <-parent.Done():
				case <-time.After(dur):
					cancel()
				}
			}()
		}
		return ctx, cancel, limits
	}
	return ctx, cancel, nil
}

func parseDurationEnv(name string) time.Duration {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
		if ms, err := strconv.Atoi(v); err == nil {
			return time.Duration(ms) * time.Millisecond
		}
	}
	return 0
}

func parseIntEnv(name string) int {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return 0
}

type bcRequest struct {
	id  int                // client ID (0..nClients-1)
	reg chan chan<- string // for reply with clientIn
	out chan<- string      // for clientOut channel (server -> client)
}

type bcServer struct {
	num       int
	join      chan *bcRequest
	clientIn  chan string
	clientOut [bcMax]chan<- string
	limits    *runLimits
}

func (s *bcServer) register(ctx context.Context, r *bcRequest) {
	idx := r.id
	chName := fmt.Sprintf("join[%d]", idx)
	s.clientOut[idx] = r.out
	s.num++
	TraceSend(ctx, "server: send to "+chName, r.reg, s.clientIn)
}

func (s *bcServer) broadcast(ctx context.Context, msg string) {
	for i := 0; i < s.num; i++ {
		chName := fmt.Sprintf("clientout[%d]", i)
		TraceSend(ctx, "server: send to "+chName, s.clientOut[i], msg)
	}
}

func (s *bcServer) recordStep() {
	if s.limits != nil {
		s.limits.step()
	}
}

// Buffered server: first register expected clients, then wait for a message and broadcast.
func (s *bcServer) runServerBuffered(ctx context.Context, expected int, onceDone chan struct{}) {
	trace.Log(ctx, "server", "server running")
	s.num = 0
	s.clientIn = make(chan string, bcMax)
	defer close(onceDone)
	// Phase 1: register all clients
	for s.num < expected {
		select {
		case <-ctx.Done():
			return
		case r := <-s.join:
			chName := fmt.Sprintf("join[%d]", r.id)
			trace.WithRegion(ctx, "server: receive from "+chName, func() {})
			s.recordStep()
			s.register(ctx, r)
		}
	}
	select {
	case <-ctx.Done():
		return
	case msg := <-s.clientIn:
		trace.WithRegion(ctx, "server: receive from clientin", func() {})
		s.recordStep()
		s.broadcast(ctx, msg)
	}
}

type bcClient struct {
	id        int
	join      chan chan<- string
	serverIn  chan string
	serverOut chan<- string
}

func (c *bcClient) register(ctx context.Context, serverJoin chan *bcRequest) {
	c.serverIn = make(chan string, 1)
	c.join = make(chan chan<- string, 1)
	req := &bcRequest{id: c.id, reg: c.join, out: c.serverIn}
	chName := fmt.Sprintf("join[%d]", c.id)
	TraceSend(ctx, "client: send to "+chName, serverJoin, req)
}

func (c *bcClient) runClient(ctx context.Context, serverJoin chan *bcRequest, gotOne *sync.WaitGroup) {
	trace.Log(ctx, "client", fmt.Sprintf("client %d running", c.id))
	c.register(ctx, serverJoin)
	defer gotOne.Done()

	sent := false
	for {
		select {
		case <-ctx.Done():
			return
		case c.serverOut = <-c.join:
			chName := fmt.Sprintf("join[%d]", c.id)
			trace.WithRegion(ctx, "client: receive from "+chName, func() {})
		case msg := <-c.serverIn:
			chName := fmt.Sprintf("clientout[%d]", c.id)
			trace.WithRegion(ctx, "client: receive from "+chName, func() {
				if trace.IsEnabled() {
					trace.Log(ctx, "value", fmt.Sprint(msg))
				}
			})
			trace.Log(ctx, "client", fmt.Sprintf("client %d received: %s", c.id, msg))
			return
		default:
			if c.id == 1 && c.serverOut != nil && !sent {
				TraceSend(ctx, "client: send to clientin", c.serverOut, "Hello World")
				trace.Log(ctx, "client", "client 1 broadcasted Hello World")
				sent = true
			}
		}
	}
}

// RunBroadcastProgram starts a server and a few clients, performs a single broadcast, and returns.
func RunBroadcastProgram(ctx context.Context) {
	ctx, cancel, limits := withBroadcastLimits(ctx)
	defer cancel()
	mode := strings.ToLower(os.Getenv("GTV_BC_MODE"))
	switch mode {
	case "blocking":
		runBroadcastBlocking(ctx, limits)
	case "both", "all":
		trace.Log(ctx, "main", "creating channels")
		runBroadcastBuffered(ctx, limits)
		time.Sleep(2 * time.Millisecond)
		trace.Log(ctx, "main", "creating channels")
		runBroadcastBlocking(ctx, limits)
	default:
		runBroadcastBuffered(ctx, limits)
	}
}

// Buffered variant: current behavior with regions/logs for pairing and synthesis.
func runBroadcastBuffered(ctx context.Context, limits *runLimits) {
	const nClients = 3
	trace.Log(ctx, "main", "creating broadcast channels")
	s := &bcServer{join: make(chan *bcRequest, bcMax), limits: limits}
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(nClients)

	// Start clients.
	for i := 0; i < nClients; i++ {
		ci := i
		go func() {
			trace.WithRegion(ctx, fmt.Sprintf("client %d goroutine", ci), func() {
				c := &bcClient{id: ci}
				c.runClient(ctx, s.join, &wg)
			})
		}()
	}

	// Run server (buffered mode waits for all clients to register).
	go s.runServerBuffered(ctx, nClients, done)

	select {
	case <-ctx.Done():
	case <-done:
	}
	wg.Wait()
}

// Blocking variant: unbuffered channels, clear phases to create strong causality edges.
func runBroadcastBlocking(ctx context.Context, limits *runLimits) {
	const nClients = 3
	trace.Log(ctx, "main", "creating broadcast channels (blocking)")
	s := &bcServer{join: make(chan *bcRequest)} // unbuffered join
	if limits != nil {
		s.limits = limits
	}
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(nClients)

	// Clients (blocking phases, unbuffered per-client inbox)
	for i := 0; i < nClients; i++ {
		ci := i
		go func() {
			c := &bcClient{id: ci}
			// Phase 1: register with unbuffered join and inbox
			c.serverIn = make(chan string)    // unbuffered inbox
			c.join = make(chan chan<- string) // unbuffered reply
			req := &bcRequest{id: c.id, reg: c.join, out: c.serverIn}
			j := fmt.Sprintf("join[%d]", c.id)
			trace.WithRegion(ctx, "client: send to "+j, func() { s.join <- req })
			trace.WithRegion(ctx, "client: receive from "+j, func() { c.serverOut = <-c.join })

			// Phase 2: client 1 sends once on unbuffered clientin
			if ci == 1 {
				trace.WithRegion(ctx, "client: send to clientin", func() { c.serverOut <- "Hello World" })
				trace.Log(ctx, "client", "client 1 broadcasted Hello World")
			}
			// Phase 3: wait for broadcast, then exit
			ch := fmt.Sprintf("clientout[%d]", c.id)
			var msg string
			trace.WithRegion(ctx, "client: receive from "+ch, func() { msg = <-c.serverIn })
			trace.Log(ctx, "client", fmt.Sprintf("client %d received: %s", c.id, msg))
			wg.Done()
		}()
	}

	// Server (unbuffered clientIn)
	s.clientIn = make(chan string)
	go func() {
		trace.Log(ctx, "server", "server running (blocking)")
		defer close(done)
		regs := 0
		for regs < nClients {
			select {
			case <-ctx.Done():
				return
			case r := <-s.join:
				j := fmt.Sprintf("join[%d]", r.id)
				trace.WithRegion(ctx, "server: receive from "+j, func() {})
				s.recordStep()
				trace.WithRegion(ctx, "server: send to "+j, func() { s.clientOut[r.id] = r.out; r.reg <- s.clientIn })
				regs++
			}
		}
		var msg string
		select {
		case <-ctx.Done():
			return
		case msg = <-s.clientIn:
			trace.WithRegion(ctx, "server: receive from clientin", func() {})
			s.recordStep()
		}
		for i := 0; i < nClients; i++ {
			ch := fmt.Sprintf("clientout[%d]", i)
			v := msg
			trace.WithRegion(ctx, "server: send to "+ch, func() { s.clientOut[i] <- v })
		}
	}()

	<-done
	wg.Wait()
}
