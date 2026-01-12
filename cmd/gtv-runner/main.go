package main

import (
	"context"
	"flag"
	"jspt/gtv"
	"jspt/internal/workload"
	"log"
	"os"
	"runtime/trace"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func main() {
	wl := flag.String("workload", "", "workload name to run (registered or built-in)")
	timeoutStr := flag.String("timeout", "", "optional run timeout (e.g., 3s); stops trace on expiry")
	flag.Parse()
	if *wl == "" {
		log.Fatal("-workload is required")
	}
	gtv.InstallStopOnSignal()

	// Start writing the runtime/trace stream to stdout. The live server reads
	// ONLY stdout as a binary trace stream. Any textual prints must go to stderr.
	if err := gtv.Start(os.Stdout); err != nil {
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

	// Optional timeout handling: set env for in-process hooks + escalation signals.
	if d, err := time.ParseDuration(*timeoutStr); err == nil && d > 0 {
		ms := int(d / time.Millisecond)
		if ms <= 0 {
			ms = 1
		}
		_ = os.Setenv("GTV_TIMEOUT_MS", strconv.Itoa(ms))
		gtv.InstallStopAfterFromEnv("GTV_TIMEOUT_MS")
		grace := 500 * time.Millisecond
		proc, _ := os.FindProcess(os.Getpid())
		go func() {
			select {
			case <-done:
				return
			case <-time.After(d + grace):
			}
			if proc != nil {
				_ = proc.Signal(os.Interrupt)
			}
			select {
			case <-done:
				return
			case <-time.After(grace):
			}
			if proc != nil {
				_ = proc.Signal(syscall.SIGTERM)
			}
			select {
			case <-done:
				return
			case <-time.After(grace):
			}
			if proc != nil {
				_ = proc.Signal(syscall.SIGKILL)
			}
		}()
	}

	<-done
	task.End()
	gtv.Stop()
	gtv.Flush()
}
