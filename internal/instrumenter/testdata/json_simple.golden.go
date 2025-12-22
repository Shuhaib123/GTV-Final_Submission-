package workload

import (
	"context"
	"encoding/json"
	"runtime/trace"
)

func f(ctx context.Context, v any) ([]byte, error) {
	return json.Marshal(v)
}
func init() {
	RegisterWorkload("demo",
		RunDemoProgram,
	)
}
