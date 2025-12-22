package workload

import (
	"context"
	"fmt"
	"runtime/trace"
	"time"
)

func f(ctx context.Context, ch chan int, out int) {
	{
		__gtvSel1 := fmt.Sprintf("s%v", time.Now().UnixNano())
		if trace.
			IsEnabled() {
			trace.Log(ctx, "select", "select_begin id="+
				__gtvSel1,
			)
		}
		select {
		case __gtvRecv1 := <-ch:
			if trace.
				IsEnabled() {
				trace.Log(ctx, "select", "select_chosen id="+
					__gtvSel1,
				)
			}
			if trace.
				IsEnabled() {
				trace.Log(ctx, "ch_ptr", fmt.Sprintf("ptr=%p",
					ch))
			}
			trace.WithRegion(ctx, "worker: receive from "+"ch", func() {
				{
					v := __gtvRecv1
					_ = v
					if trace.
						IsEnabled() {
						trace.Log(ctx, "value", fmt.Sprint(__gtvRecv1))
					}
				}
			})

		}
		if trace.
			IsEnabled() {
			trace.Log(ctx, "select", "select_end id="+
				__gtvSel1,
			)
		}
	}
	{
		__gtvSel2 := fmt.Sprintf("s%v", time.Now().UnixNano())
		if trace.
			IsEnabled() {
			trace.Log(ctx, "select", "select_begin id="+
				__gtvSel2,
			)
		}

		select {
		case ch <- out:
			if trace.
				IsEnabled() {
				trace.Log(ctx, "select", "select_chosen id="+
					__gtvSel2,
				)
			}
			if trace.
				IsEnabled() {
				trace.Log(ctx, "ch_ptr", fmt.Sprintf("ptr=%p",
					ch))
			}
			trace.WithRegion(ctx, "worker: send to "+"ch", func() {
				{
				}
			})

		}
		if trace.
			IsEnabled() {
			trace.Log(ctx, "select", "select_end id="+
				__gtvSel2,
			)
		}
	}

}
func init() {
	RegisterWorkload("demo", RunDemoProgram)
}
