/* package main

import (
	"fmt"
	"log"
	"os"
	"runtime/trace"
	"sync"
)

func main() {
	f, err := os.Create("trace.out")
	if err != nil {
		log.Fatalf("failed to create trace file: %v", err)
	}
	defer f.Close()

	if err := trace.Start(f); err != nil {
		log.Fatalf("failed to start trace: %v", err)
	}
	defer trace.Stop()

	var wg sync.WaitGroup
	ch := make(chan string)

	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 5; i++ {
			msg := <-ch
			fmt.Println("Ping received:", msg)
			ch <- "pong"
		}
	}()

	go func() {
		defer wg.Done()
		ch <- "ping"
		for i := 0; i < 5; i++ {
			msg := <-ch
			fmt.Println("Pong received:", msg)
			if i < 4 {
				ch <- "ping"
			}
		}
	}()

	wg.Wait()
}
*/

package main

import (
	"context"
	"flag"
	"jspt/internal/gtvtrace"
	"jspt/internal/workload"
	"log"
	"os"
	"runtime/trace"
	"strings"
)

func main() {
	parseTrace := flag.String("parse", "", "trace file to convert into timeline JSON")
	parseJSON := flag.String("json", "trace.json", "output path for the generated timeline JSON")
	flag.Parse()

	if *parseTrace != "" {
		if err := WritePingPongTimelineJSON(*parseTrace, *parseJSON); err != nil {
			log.Fatalf("failed to parse %s: %v", *parseTrace, err)
		}
		log.Printf("parsed %s → %s", *parseTrace, *parseJSON)
		return
	}

	if err := gtvtrace.StartFile("trace.out"); err != nil {
		log.Fatalf("failed to create trace file: %v", err)
	}
	gtvtrace.SetFlushHook(func() error {
		return WritePingPongTimelineJSON("trace.out", *parseJSON)
	})
	gtvtrace.InstallStopOnSignal()
	gtvtrace.InstallStopAfterFromEnv("GTV_TIMEOUT_MS")

	// Choose workload from env: GTV_WORKLOAD=pingpong|broadcast|skipgraph_full|mergesort (default: pingpong)
	wl := os.Getenv("GTV_WORKLOAD")
	if wl == "" {
		wl = "pingpong"
	}
	wl = strings.ToLower(wl)

	ctx, task := trace.NewTask(context.Background(), strings.Title(wl))
	defer gtvtrace.FlushIdempotent()
	defer gtvtrace.StopIdempotent()
	defer task.End()

	// First, allow dynamically registered workloads.
	if workload.RunByName(ctx, wl) {
		return
	}

	switch wl {
	case "broadcast":
		workload.RunBroadcastProgram(ctx)
	case "skipgraph_full":
		workload.RunSkipGraphFullProgram(ctx)
	case "mergesort":
		workload.RunMergeSortProgram(ctx)
	default:
		workload.RunPingPongProgram(ctx)
	}
	// Stop/flush handled by deferred gtvtrace calls.
}
