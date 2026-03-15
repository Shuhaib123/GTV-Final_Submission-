//go:build workload_bdcst1231
// +build workload_bdcst1231

// file: cmd/gtv_demo/main.go
package workload

import (
	"fmt"
	"sync"
	"context"
	"runtime/trace"
	"jspt/internal/gtvtrace"
	"sync/atomic"
)

var __jspt_spawn_id_2 uint64

type Job struct {
	ID int
}

type Result struct {
	JobID	int
	Value	int
}

func producer(__jspt_ctx_10 context.Context, jobs chan<- Job, n int) {
	// gtv:send=jobs
	for i := 0; i < n; i++ {
		if trace.IsEnabled() {
			trace.
				Log(__jspt_ctx_10,

					"ch_ptr",

					fmt.Sprintf("ptr=%p",

						jobs,
					))
		}
		trace.WithRegion(__jspt_ctx_10, "worker: send to jobs", func() {
			{

				jobs <- Job{ID: i}
			}
		})
	}
	trace.WithRegion(__jspt_ctx_10, "chan.close", func() {
		{
			if trace.IsEnabled() {
				trace.
					Log(__jspt_ctx_10,

						"chan_close",

						fmt.Sprintf("ptr=%p",

							jobs))
			}

			close(jobs)
		}
	})
}

func worker(__jspt_ctx_11 context.Context, workerID int, jobs <-chan Job, results chan<- Result, wg *sync.WaitGroup) {
	defer func() {
		trace.WithRegion(

		// gtv:recv=jobs
		__jspt_ctx_11, "wg.done wg", func() {
			{
				wg.Done()
			}
		})
	}()

	for {

		job, ok := __jspt_recv_value_ok_7(__jspt_ctx_11,

		// Some deterministic work; keep it simple for clearer traces.
		"worker: receive from jobs", jobs)

		if !ok {
			return
		}

		val := (job.ID + 1) * (job.ID + 1)
		if trace.IsEnabled() {
			trace.
				Log(__jspt_ctx_11,

					"ch_ptr",

					fmt.Sprintf("ptr=%p",

						results,
					))
		}
		trace.WithRegion(__jspt_ctx_11, "worker: send to results", func() {
			{

				// gtv:send=results
				results <- Result{JobID: job.ID, Value: val + workerID}
			}
		})
	}
}

func closeWhenDone(__jspt_ctx_12 context.Context, results chan<- Result, wg *sync.WaitGroup) {
	trace.WithRegion(__jspt_ctx_12, "wg.wait wg", func() {
		{
			wg.Wait()
		}
	})
	trace.WithRegion(__jspt_ctx_12, "chan.close", func() {
		{
			if trace.IsEnabled() {
				trace.
					Log(__jspt_ctx_12,

						"chan_close",

						fmt.Sprintf("ptr=%p",

							results))
			}

			close(results)
		}
	})
}

func aggregator(__jspt_ctx_13 context.Context, results <-chan Result, done chan<- int) {
	sum := 0
	for {
		// gtv:recv=results
		r, ok := __jspt_recv_value_ok_7(__jspt_ctx_13,

		// gtv:send=done
		"worker: receive from results", results)

		if !ok {
			if trace.IsEnabled() {
				trace.
					Log(__jspt_ctx_13,

						"ch_ptr",

						fmt.Sprintf("ptr=%p",

							done,
						))
			}
			trace.WithRegion(__jspt_ctx_13, "worker: send to done", func() {
				{

					done <- sum
				}
			})
			return
		}
		sum += r.Value
	}
}

func Runbdcst1231Program(__jspt_ctx_8 context.Context) {
	var __jspt_wg_0 sync.WaitGroup
	defer __jspt_wg_0.Wait()
	gtvtrace.InstallStopOnSignal()
	gtvtrace.InstallStopAfterFromEnv("GTV_TIMEOUT_MS",
	)
	__jspt_ctx_8, __jspt_task_9 := trace.NewTask(__jspt_ctx_8, "bdcst1231")
	defer __jspt_task_9.End()
	trace.Log(__jspt_ctx_8, "main", "bdcst1231 starting")

	const (
		numJobs		= 25
		numWorkers	= 4
	)

	// Unbuffered channels maximize visible blocking + edges in most tracers.
	jobs := make(chan Job)
	if trace.IsEnabled() {
		trace.
			Log(__jspt_ctx_8,

				"chan_make",

				fmt.Sprintf("ptr=%p cap=%d type=%s",

					jobs,
					0, "Job"))
	}

	results := make(chan Result)
	if trace.IsEnabled() {
		trace.
			Log(__jspt_ctx_8,

				"chan_make",

				fmt.Sprintf("ptr=%p cap=%d type=%s",

					results,
					0, "Result",
				))
	}

	done := make(chan int)
	if trace.IsEnabled() {
		trace.
			Log(__jspt_ctx_8,

				"chan_make",

				fmt.Sprintf("ptr=%p cap=%d type=%s",

					done,
					0, "int"))
	}

	var wg sync.WaitGroup
	trace.WithRegion(__jspt_ctx_8, "wg.add wg", func() {
		{
			wg.Add(numWorkers)
		}
	})
	__jspt_spawn_14 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
	trace.Log(__jspt_ctx_8, "spawn_parent",
		fmt.
			Sprintf("sid=%d",

				__jspt_spawn_14,
			))
	__jspt_wg_0.Add(1)
	go func(__jspt_ctx_18 context.Context) {
		trace.WithRegion(__jspt_ctx_18, "goroutine: anon", func() {
			{
				defer __jspt_wg_0.Done()
				trace.Log(__jspt_ctx_8, "spawn_child",
					fmt.
						Sprintf("sid=%d",

							__jspt_spawn_14,
						))

				producer(__jspt_ctx_8, jobs, numJobs)
			}
		})
	}(__jspt_ctx_8)

	for wid := 0; wid < numWorkers; wid++ {
		__jspt_spawn_15 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
		trace.Log(__jspt_ctx_8, "spawn_parent",
			fmt.
				Sprintf("sid=%d",

					__jspt_spawn_15,
				))
		__jspt_wg_0.Add(1)
		go func(__jspt_ctx_19 context.Context) {
			trace.WithRegion(__jspt_ctx_19, "goroutine: anon", func() {
				{
					defer __jspt_wg_0.Done()
					trace.Log(__jspt_ctx_8, "spawn_child",
						fmt.
							Sprintf("sid=%d",

								__jspt_spawn_15,
							))

					worker(__jspt_ctx_8, wid, jobs, results, &wg)
				}
			})
		}(__jspt_ctx_8)
	}
	__jspt_spawn_16 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
	trace.Log(__jspt_ctx_8, "spawn_parent",
		fmt.
			Sprintf("sid=%d",

				__jspt_spawn_16,
			))
	__jspt_wg_0.Add(1)
	go func(__jspt_ctx_20 context.Context) {
		trace.WithRegion(__jspt_ctx_20, "goroutine: anon", func() {
			{
				defer __jspt_wg_0.Done()
				trace.Log(__jspt_ctx_8, "spawn_child",
					fmt.
						Sprintf("sid=%d",

							__jspt_spawn_16,
						))

				closeWhenDone(__jspt_ctx_8, results, &wg)
			}
		})
	}(__jspt_ctx_8)
	__jspt_spawn_17 :=

	// gtv:recv=done
	atomic.AddUint64(&__jspt_spawn_id_2, 1)
	trace.Log(__jspt_ctx_8, "spawn_parent",
		fmt.
			Sprintf("sid=%d",

				__jspt_spawn_17,
			))
	__jspt_wg_0.Add(1)
	go func(__jspt_ctx_21 context.Context) {
		trace.WithRegion(__jspt_ctx_21, "goroutine: anon", func() {
			{
				defer __jspt_wg_0.Done()
				trace.Log(__jspt_ctx_8, "spawn_child",
					fmt.
						Sprintf("sid=%d",

							__jspt_spawn_17,
						))

				aggregator(__jspt_ctx_8, results, done)
			}
		})
	}(__jspt_ctx_8)

	total := __jspt_recv_value_6(__jspt_ctx_8, "worker: receive from done", done)

	fmt.Println("sum =", total)
}
func __jspt_recv_value_6[T any](__jspt_ctx_22 context.
	Context, label string, ch <-chan T) T {
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_22,
			"ch_ptr", fmt.
				Sprintf("ptr=%p", ch),
		)
		trace.Log(__jspt_ctx_22,

			"ch_name", fmt.Sprintf("ptr=%p name=%s", ch, label))
	}
	var v T
	trace.WithRegion(__jspt_ctx_22,

		label,
		func() {
			v = <-ch
		})
	return v
}
func __jspt_recv_value_ok_7[T any](__jspt_ctx_22 context.Context, label string, ch <-chan T) (T, bool) {
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_22,
			"ch_ptr", fmt.Sprintf("ptr=%p", ch))
		trace.Log(__jspt_ctx_22,

			"ch_name", fmt.Sprintf("ptr=%p name=%s", ch, label))
	}
	var v T
	var ok bool
	trace.
		WithRegion(__jspt_ctx_22, label, func() { v, ok = <-ch })
	return v, ok
}
func init() {
	RegisterWorkload("bdcst1231",
		Runbdcst1231Program,
	)
}
