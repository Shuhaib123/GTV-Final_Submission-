//go:build workload_fan
// +build workload_fan

package workload

import (
	"context"
	"fmt"
	"sync"
	"time"
	"runtime/trace"
	"jspt/internal/gtvtrace"
	"sync/atomic"
)

var __jspt_spawn_id_2 uint64

type item struct {
	id	int
	val	int
}

func RunfanProgram(__jspt_ctx_8 context.Context) {
	var __jspt_wg_0 sync.WaitGroup
	defer __jspt_wg_0.Wait()
	gtvtrace.InstallStopOnSignal()
	gtvtrace.InstallStopAfterFromEnv("GTV_TIMEOUT_MS",
	)
	__jspt_ctx_8, __jspt_task_9 := trace.NewTask(__jspt_ctx_8, "fan")
	defer __jspt_task_9.End()
	trace.Log(__jspt_ctx_8, "main", "fan starting")

	ctx, cancel := context.WithTimeout(context.Background(), 350*time.Millisecond)
	defer cancel()

	in := make(chan item)
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_8,

			"chan_make",
			fmt.Sprintf("ptr=%p cap=%d type=%s",

				in, 0, "item"))
	}

	out := make(chan item)
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_8,

			"chan_make",
			fmt.Sprintf("ptr=%p cap=%d type=%s",

				out, 0, "item"))
	}

	done := make(chan int, 1)
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_8,

			"chan_make",
			fmt.Sprintf("ptr=%p cap=%d type=%s",

				done, 1, "int"))
	}
	__jspt_spawn_10 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
	trace.Log(__jspt_ctx_8,

		"spawn_parent",
		fmt.
			Sprintf("sid=%d",

				__jspt_spawn_10))
	__jspt_wg_0.Add(1)
	go

	// Stage 1: generator
	func(__jspt_ctx_14 context.Context,) {
		trace.WithRegion(__jspt_ctx_14, "goroutine: anon", func() {
			{
				defer __jspt_wg_0.Done()
				trace.Log(__jspt_ctx_8,

					"spawn_child",
					fmt.
						Sprintf("sid=%d",

							__jspt_spawn_10))

				defer close(in)
				for i := 0; i < 20; i++ {
					select {
					case <-ctx.Done():
						return
					case in <- item{id: i, val: i * 2}:
						time.Sleep(8 * time.Millisecond)
					}
				}
			}
		})
	}(__jspt_ctx_8)

	// Stage 2: fan-out workers
	const workers = 3
	var wg sync.WaitGroup
	wg.Add(workers)

	for w := 0; w < workers; w++ {
		wid := w
		__jspt_spawn_11 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
		trace.Log(__jspt_ctx_8,

			"spawn_parent",
			fmt.
				Sprintf("sid=%d",

					__jspt_spawn_11))
		__jspt_wg_0.Add(1)
		go func(__jspt_ctx_15 context.Context,) {
			trace.WithRegion(__jspt_ctx_15, "goroutine: anon", func() {
				{
					defer __jspt_wg_0.Done()
					trace.Log(__jspt_ctx_8,

						"spawn_child",
						fmt.
							Sprintf("sid=%d",

								__jspt_spawn_11))

					defer wg.Done()
					for it := range in {
						select {
						case <-ctx.Done():
							return
						case <-time.After(time.Duration(10+it.id%7) * time.Millisecond):
							it.val += wid
						}

						select {
						case <-ctx.Done():
							return
						case out <- it:
						}
					}
				}
			})
		}(__jspt_ctx_8)

	}
	__jspt_spawn_12 :=

	// Close out when workers finish
	atomic.AddUint64(&__jspt_spawn_id_2, 1)
	trace.Log(__jspt_ctx_8,

		"spawn_parent",
		fmt.
			Sprintf("sid=%d",

				__jspt_spawn_12))
	__jspt_wg_0.Add(1)
	go func(__jspt_ctx_16 context.Context,) {
		trace.WithRegion(__jspt_ctx_16, "goroutine: anon",

		// Stage 3: aggregator
		func() {
			{
				defer __jspt_wg_0.Done()
				trace.Log(__jspt_ctx_8,

					"spawn_child",
					fmt.
						Sprintf("sid=%d",

							__jspt_spawn_12))

				wg.Wait()
				close(out)
			}
		})
	}(__jspt_ctx_8)
	__jspt_spawn_13 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
	trace.Log(__jspt_ctx_8,

		"spawn_parent",
		fmt.
			Sprintf("sid=%d",

				__jspt_spawn_13))
	__jspt_wg_0.Add(1)
	go func(__jspt_ctx_17 context.Context,) {
		trace.WithRegion(__jspt_ctx_17, "goroutine: anon", func() {
			{
				defer __jspt_wg_0.Done()
				trace.Log(__jspt_ctx_8,

					"spawn_child",
					fmt.
						Sprintf("sid=%d",

							__jspt_spawn_13))

				sum := 0
				for {
					select {
					case <-ctx.Done():
						done <- sum
						return
					case it, ok := <-out:
						if !ok {
							done <- sum
							return
						}
						sum += it.val
					}
				}
			}
		})
	}(__jspt_ctx_8)

	fmt.Println("sum =", <-done)
}
func init() {
	RegisterWorkload("fan",
		RunfanProgram,
	)
}
