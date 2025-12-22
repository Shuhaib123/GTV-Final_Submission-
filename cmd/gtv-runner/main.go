package main

import (
	"context"
	"flag"
	"jspt/internal/workload"
	"log"
	"os"
	"runtime/trace"
	"strings"
	"time"
)

func main() {
	wl := flag.String("workload", "", "workload name to run (registered or built-in)")
	timeoutStr := flag.String("timeout", "", "optional run timeout (e.g., 3s); stops trace on expiry")
	flag.Parse()
	if *wl == "" {
		log.Fatal("-workload is required")
	}

	// Start writing the runtime/trace stream to stdout. The live server reads
	// ONLY stdout as a binary trace stream. Any textual prints must go to stderr.
	if err := trace.Start(os.Stdout); err != nil {
		log.Fatal(err)
	}
	// Redirect all subsequent fmt.Print* and default log output to stderr so
	// they do not corrupt the binary trace stream on stdout.
	os.Stdout = os.Stderr
	log.SetOutput(os.Stderr)
	log.Printf("gtv-runner: requested workload=%q", *wl)
	ctx, task := trace.NewTask(context.Background(), strings.Title(*wl))
	done := make(chan struct{})
	go func() {
		// Try dynamic workloads first
		if !workload.RunByName(ctx, *wl) {
			log.Printf("gtv-runner: RunByName miss, falling back to built-ins")
			switch *wl {
			case "broadcast":
				workload.RunBroadcastProgram(ctx)
			case "skipgraph_full":
				workload.RunSkipGraphFullProgram(ctx)
			case "mergesort":
				workload.RunMergeSortProgram(ctx)
			case "recvonly":
				workload.RunRecvOnlyProgram(ctx)
			default:
				workload.RunPingPongProgram(ctx)
			}
		} else {
			log.Printf("gtv-runner: RunByName matched, running registered workload")
		}
		close(done)
	}()

	// Optional timeout handling
	if d, err := time.ParseDuration(*timeoutStr); err == nil && d > 0 {
		select {
		case <-done:
		case <-time.After(d):
			log.Printf("gtv-runner: timeout %s reached; stopping trace", d)
		}
	} else {
		<-done
	}
	task.End()
	trace.Stop()
}
