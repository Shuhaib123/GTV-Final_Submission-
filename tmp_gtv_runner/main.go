package main
import (
	"context"
	"os"
	"time"
	"runtime/trace"
	"log"
	"jspt/internal/workload"
)
func main(){
	tf := os.Getenv("GTV_TRACE_FILE")
	var f *os.File
	var err error
	if tf != "" {
		f, err = os.Create(tf)
		if err != nil {
			log.Fatalf("create trace file: %v", err)
		}
		if err := trace.Start(f); err != nil {
			log.Fatalf("trace start: %v", err)
		}
		defer func(){ trace.Stop(); f.Close() }()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	workload.RunbdcstProgram(ctx)
}
