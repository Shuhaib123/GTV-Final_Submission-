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
	"time"
)

func main() {
	wl := flag.String("workload", "", "workload name to run (registered or built-in)")
	timeoutStr := flag.String("timeout", "", "optional run timeout (e.g., 3s); stops trace on expiry")
	flag.Parse()
	if *wl == "" {
		log.Fatal("-workload is required")
	}

	// Start writing the runtime/trace stream. Prefer a dedicated file if provided.
	traceOut := strings.TrimSpace(os.Getenv("GTV_TRACE_OUT"))
	if traceOut != "" {
		if err := gtvtrace.StartFile(traceOut); err != nil {
			log.Fatal(err)
		}
	} else if err := gtvtrace.Start(os.Stdout); err != nil {
		log.Fatal(err)
	}
	gtvtrace.InstallStopOnSignal()
	gtvtrace.InstallStopAfterFromEnv("GTV_TIMEOUT_MS")
	// Redirect all subsequent fmt.Print* and default log output to stderr so
	// they do not corrupt the binary trace stream on stdout.
	os.Stdout = os.Stderr
	log.SetOutput(os.Stderr)
	log.Printf("gtv-runner: requested workload=%q", *wl)
	mvpMode := os.Getenv("GTV_MVP") == "1"
	ctx, task := trace.NewTask(context.Background(), strings.Title(*wl))
	defer gtvtrace.FlushIdempotent()
	defer gtvtrace.StopIdempotent()
	done := make(chan struct{})
	go func() {
		// Try dynamic workloads first
		if !workload.RunByName(ctx, *wl) {
			log.Printf("gtv-runner: RunByName miss, falling back to built-ins")
			switch *wl {
			case "broadcast":
				workload.RunBroadcastProgram(ctx)
			case "skipgraph_full":
				if mvpMode {
					log.Printf("gtv-runner: MVP mode active; falling back to pingpong")
					workload.RunPingPongProgram(ctx)
					break
				}
				workload.RunSkipGraphFullProgram(ctx)
			case "mergesort":
				if mvpMode {
					log.Printf("gtv-runner: MVP mode active; falling back to pingpong")
					workload.RunPingPongProgram(ctx)
					break
				}
				workload.RunMergeSortProgram(ctx)
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
			task.End()
			gtvtrace.StopFlushExit(0)
		}
	} else {
		<-done
	}
	task.End()
}
