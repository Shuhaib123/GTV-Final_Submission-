package workload

import (
	"context"
	"log"
)

// Simple runtime registry so dynamically generated workloads can register
// themselves without changing central switches.

var registry = make(map[string]func(context.Context))

func RegisterWorkload(name string, fn func(context.Context)) {
	if name == "" || fn == nil {
		return
	}
	log.Println("RegisterWorkload:", name)
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
