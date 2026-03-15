//go:build workload_mutex
// +build workload_mutex

package workload

import (
	"fmt"
	"sync"
	"time"
	"context"
	"runtime/trace"
	"jspt/internal/gtvtrace"
	"sync/atomic"
)

var __jspt_spawn_id_2 uint64

func RunmutexProgram(__jspt_ctx_8 context.Context) {
	var __jspt_wg_0 sync.WaitGroup
	defer __jspt_wg_0.Wait(

	// A channel to demonstrate Block/Unblock events
	)
	gtvtrace.InstallStopOnSignal()
	gtvtrace.InstallStopAfterFromEnv("GTV_TIMEOUT_MS",
	)
	__jspt_ctx_8, __jspt_task_9 := trace.NewTask(__jspt_ctx_8, "mutex")
	defer __jspt_task_9.End()
	trace.Log(__jspt_ctx_8, "main", "mutex starting")
	var wg sync.WaitGroup
	var mu sync.Mutex

	ch := make(chan string)
	if trace.IsEnabled() {
		trace.
			Log(__jspt_ctx_8,
				"chan_make", fmt.Sprintf("ptr=%p cap=%d type=%s",

					ch, 0, "string"))
	}

	fmt.Println("Starting demonstration...")
	trace.

	// 1. WaitGroup Event: Adding tasks
	WithRegion(__jspt_ctx_8, "wg.add wg", func() {
		{

			wg.Add(3)
		}
	})
	__jspt_spawn_10 :=

	// GOROUTINE 1: Mutex Contention & Channel Sending
	atomic.AddUint64(&__jspt_spawn_id_2, 1)
	trace.Log(__jspt_ctx_8,

		"spawn_parent",

		fmt.Sprintf("sid=%d", __jspt_spawn_10,
		))
	__jspt_wg_0.Add(1)
	go func(__jspt_ctx_13 context.Context,) {
		trace.WithRegion(__jspt_ctx_13, "goroutine: anon", func() {
			{
				defer __jspt_wg_0.Done()
				trace.Log(__jspt_ctx_8,

					"spawn_child",

					fmt.Sprintf("sid=%d", __jspt_spawn_10,
					))

				defer wg.Done()

				fmt.Println("G1: Attempting to lock Mutex...")
				trace.WithRegion(__jspt_ctx_8, "mutex.lock mu", func() {
					{
						mu.Lock()
					}
				})
				fmt.Println("G1: Mutex locked. Working...")
				time.Sleep(2 * time.Second)
				trace.WithRegion(__jspt_ctx_8, "mutex.unlock mu", func() {
					{
						mu.Unlock()
					}
				})
				fmt.Println("G1: Mutex released.")

				fmt.Println("G1: Blocking on channel send...")
				if trace.IsEnabled() {
					trace.
						Log(__jspt_ctx_8,
							"value", fmt.Sprint(
								"Data from G1"))
				}
				if trace.IsEnabled() {
					trace.
						Log(__jspt_ctx_8,
							"ch_ptr", fmt.Sprintf("ptr=%p", ch))
				}
				trace.WithRegion(__jspt_ctx_8, "worker: send to "+"ch", func() {
					{

						ch <- "Data from G1"
					}
				})	// Blocks until G3 receives
				fmt.Println("G1: Unblocked and finished.")
			}
		})
	}(__jspt_ctx_8)

	// GOROUTINE 2: Mutex Blocking
	__jspt_spawn_11 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
	trace.Log(__jspt_ctx_8,

		"spawn_parent",

		fmt.Sprintf("sid=%d", __jspt_spawn_11,
		))
	__jspt_wg_0.Add(1)
	go func(__jspt_ctx_14 context.Context,) {
		trace.WithRegion(__jspt_ctx_14,

		// This will likely block because G1 holds the lock
		"goroutine: anon", func() {
			{
				defer __jspt_wg_0.Done()
				trace.Log(__jspt_ctx_8,

					"spawn_child",

					fmt.Sprintf("sid=%d", __jspt_spawn_11,
					))

				defer wg.Done()

				time.Sleep(500 * time.Millisecond)
				fmt.Println("G2: Attempting to lock Mutex (should block)...")
				trace.WithRegion(__jspt_ctx_8, "mutex.lock mu", func() {
					{
						mu.Lock()
					}
				})
				fmt.Println("G2: Finally acquired Mutex!")
				trace.WithRegion(__jspt_ctx_8,

				// GOROUTINE 3: Channel Blocking & Unblocking
				"mutex.unlock mu", func() {
					{
						mu.Unlock()
					}
				})
			}
		})
	}(__jspt_ctx_8)
	__jspt_spawn_12 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
	trace.Log(__jspt_ctx_8,

		"spawn_parent",

		fmt.Sprintf("sid=%d", __jspt_spawn_12,
		))
	__jspt_wg_0.Add(1)
	go func(__jspt_ctx_15 context.Context,) {
		trace.WithRegion(__jspt_ctx_15, "goroutine: anon", func() {
			{
				defer __jspt_wg_0.Done()
				trace.Log(__jspt_ctx_8,

					"spawn_child",

					fmt.Sprintf("sid=%d", __jspt_spawn_12,
					))

				defer wg.Done()

				fmt.Println("G3: Sleeping before receiving...")
				time.Sleep(4 * time.Second)

				fmt.Println("G3: Attempting to receive from channel (Unblocking G1)...")
				msg := __jspt_recv_value_6(__jspt_ctx_8, "worker: receive from "+

				// 2. Wait Event: Main goroutine waits for all others
				"ch", ch)

				fmt.Println("G3: Received:", msg)
			}
		})
	}(__jspt_ctx_8)

	fmt.Println("Main: Waiting for all goroutines to finish...")
	trace.WithRegion(__jspt_ctx_8, "wg.wait wg", func() {
		{
			wg.Wait()
		}
	})
	fmt.Println("Main: All events complete.")
}
func __jspt_recv_value_6[T any](__jspt_ctx_16 context.Context, label string, ch <-chan T) T {
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_16, "ch_ptr",

			fmt.Sprintf("ptr=%p", ch))
		trace.Log(__jspt_ctx_16,

			"ch_name", fmt.Sprintf("ptr=%p name=%s", ch, label,
			))
	}
	var v T
	trace.WithRegion(__jspt_ctx_16, label, func() { v = <-ch })
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_16, "value",
			fmt.Sprint(v))
	}
	return v
}
func __jspt_recv_value_ok_7[T any](__jspt_ctx_16 context.
	Context, label string, ch <-chan T) (T, bool) {
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_16,
			"ch_ptr", fmt.Sprintf("ptr=%p", ch))
		trace.
			Log(__jspt_ctx_16, "ch_name", fmt.
				Sprintf("ptr=%p name=%s",
					ch,
					label,
				))
	}
	var v T
	var ok bool
	trace.WithRegion(__jspt_ctx_16,
		label, func() {
			v, ok = <-ch
		})
	if ok && trace.IsEnabled() {
		trace.Log(__jspt_ctx_16, "value",
			fmt.Sprint(v))
	}
	return v, ok
}
func init() {
	RegisterWorkload("mutex",
		RunmutexProgram,
	)
}
