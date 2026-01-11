//go:build workload_pipeline
// +build workload_pipeline

package workload

import (
	"context"
	"fmt"
	"sync"
	"time"
	"runtime/trace"
	"sync/atomic"
)

var __jspt_spawn_id_2 uint64

type item struct {
	id	int
	val	int
}

func RunPipelineProgram(__jspt_ctx_5 context.Context) {
	var __jspt_wg_0 sync.WaitGroup
	defer __jspt_wg_0.Wait()
	__jspt_ctx_5, __jspt_task_6 := trace.NewTask(__jspt_ctx_5, "Pipeline")
	defer __jspt_task_6.End()
	trace.Log(__jspt_ctx_5, "main", "Pipeline starting")
	ctx, cancel := context.WithTimeout(context.Background(), 350*time.Millisecond)
	defer cancel()

	in := make(chan item)
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_5,

			"chan_make",
			fmt.Sprintf("ptr=%p cap=%d type=%s",

				in, 0, "item"))
	}

	out := make(chan item)
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_5,

			"chan_make",
			fmt.Sprintf("ptr=%p cap=%d type=%s",

				out, 0, "item"))
	}

	done := make(chan int, 1)
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_5,

			"chan_make",
			fmt.Sprintf("ptr=%p cap=%d type=%s",

				done, 1, "int"))
	}
	__jspt_spawn_7 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
	trace.Log(__jspt_ctx_5,

		"spawn_parent",
		fmt.
			Sprintf("sid=%d",

				__jspt_spawn_7))
	__jspt_wg_0.Add(1)
	go

	// Stage 1: generator
	func(__jspt_ctx_11 context.Context,) {
		defer __jspt_wg_0.Done()
		trace.Log(__jspt_ctx_5,

			"spawn_child",
			fmt.
				Sprintf("sid=%d",

					__jspt_spawn_7))

		defer close(in)
		for i := 0; i < 20; i++ {
			select {
			case <-ctx.Done():
				return
			case in <- item{id: i, val: i * 2}:
				time.Sleep(8 * time.Millisecond)
			}
		}
	}(__jspt_ctx_5)

	// Stage 2: fan-out workers
	const workers = 3
	var wg sync.WaitGroup
	wg.Add(workers)

	for w := 0; w < workers; w++ {
		wid := w
		__jspt_spawn_8 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
		trace.Log(__jspt_ctx_5,

			"spawn_parent",
			fmt.
				Sprintf("sid=%d",

					__jspt_spawn_8))
		__jspt_wg_0.Add(1)
		go func(__jspt_ctx_12 context.Context,) {
			defer __jspt_wg_0.Done()
			trace.Log(__jspt_ctx_5,

				"spawn_child",
				fmt.
					Sprintf("sid=%d",

						__jspt_spawn_8))

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
		}(__jspt_ctx_5)

	}
	__jspt_spawn_9 :=

	// Close out when workers finish
	atomic.AddUint64(&__jspt_spawn_id_2, 1)
	trace.Log(__jspt_ctx_5,

		"spawn_parent",
		fmt.
			Sprintf("sid=%d",

				__jspt_spawn_9))
	__jspt_wg_0.Add(1)
	go func(__jspt_ctx_13 context.Context,) {
		defer __jspt_wg_0.Done()
		trace.Log(__jspt_ctx_5,

			"spawn_child",
			fmt.
				Sprintf("sid=%d",

					__jspt_spawn_9))

		wg.Wait()
		close(out)
	}(__jspt_ctx_5)

	// Stage 3: aggregator
	__jspt_spawn_10 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
	trace.Log(__jspt_ctx_5,

		"spawn_parent",
		fmt.
			Sprintf("sid=%d",

				__jspt_spawn_10))
	__jspt_wg_0.Add(1)
	go func(__jspt_ctx_14 context.Context,) {
		defer __jspt_wg_0.Done()
		trace.Log(__jspt_ctx_5,

			"spawn_child",
			fmt.
				Sprintf("sid=%d",

					__jspt_spawn_10))

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
	}(__jspt_ctx_5)

	fmt.Println("sum =", <-done)
}
func init() {
	RegisterWorkload("pipeline",
		RunPipelineProgram,
	)
}
