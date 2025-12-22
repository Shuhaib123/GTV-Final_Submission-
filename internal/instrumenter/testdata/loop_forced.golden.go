package workload

import (
	"context"
	"runtime/trace"
	// gtv:loop=hot
)

func f(ctx context.Context, xs []int) {
	for _, x := range xs {
		var __gtvBreak bool
		var __gtvCont bool
		var __gtvReturn bool
		trace.WithRegion(ctx, "loop:hot", func() {
			{

				if x%2 == 0 {
					{
						__gtvCont = true
						return
					}
				}
				if x > 10 {
					{
						__gtvBreak = true
						return
					}
				}
			}
		})
		if __gtvBreak {
			break
		}
		if __gtvCont {
			continue
		}
		if __gtvReturn {
			return
		}

	}
}
func init() {
	RegisterWorkload("demo", RunDemoProgram)
}
