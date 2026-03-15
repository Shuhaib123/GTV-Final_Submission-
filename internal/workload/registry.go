package workload

import (
	"context"
	"log"
	"os"
)

// Simple runtime registry so dynamically generated workloads can register
// themselves without changing central switches.

var registry = make(map[string]func(context.Context))
var regLogger = log.New(os.Stderr, "", log.LstdFlags)
func RegisterWorkload(name string, fn func(context.Context)) {
	if name == "" || fn == nil {
		return
	}
	regLogger.Println("RegisterWorkload:", name)
	registry[name] = fn
}

// RunByName returns true if a workload was found and executed.
func RunByName(ctx context.Context, name string) bool {
	if fn, ok := registry[name]; ok {
		fn(ctx)
		return true
	}
	return false
}
