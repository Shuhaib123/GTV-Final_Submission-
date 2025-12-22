package main
import (
	"context"
	"log"
	"os"
	"runtime/trace"
	"time"

	"jspt/internal/workload"
)
func main() {
	tf := os.Getenv("GTV_TRACE_FILE")
	var f *os.File
	var err error
	if tf != "" {
		f, err = os.Create(tf)
		if err != nil {
			log.Fatalf("create trace: %v", err)
		}
		if err := trace.Start(f); err != nil {
			log.Fatalf("trace start: %v", err)
		}
		defer func() { trace.Stop(); f.Close() }()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if !workload.RunByName(ctx, "dynamic") {
		log.Printf("workload dynamic not found", "dynamic")
	}
}
