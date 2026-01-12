//go:build workload_crdntr
// +build workload_crdntr

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

type msg struct {
	from	int
	phase	int
}

func RuncrdntrProgram(__jspt_ctx_5 context.Context) {
	var __jspt_wg_0 sync.WaitGroup
	defer __jspt_wg_0.Wait()
	__jspt_ctx_5, __jspt_task_6 := trace.NewTask(__jspt_ctx_5, "crdntr")
	defer __jspt_task_6.End()
	trace.Log(__jspt_ctx_5, "main", "crdntr starting")
	ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
	defer cancel()

	const workers = 4
	const phases = 3

	updates := make(chan msg, 32)
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_5,

			"chan_make",
			fmt.Sprintf("ptr=%p cap=%d type=%s",

				updates,
				32, "msg"))
	}

	var mu sync.Mutex
	cond := sync.NewCond(&mu)
	currentPhase := 0
	arrived := 0
	__jspt_spawn_7 :=

	// Coordinator: advances phase when all workers arrive.
	atomic.AddUint64(&__jspt_spawn_id_2, 1)
	trace.Log(__jspt_ctx_5,

		"spawn_parent",
		fmt.
			Sprintf("sid=%d",

				__jspt_spawn_7))
	__jspt_wg_0.Add(1)
	go func(__jspt_ctx_10 context.Context,) {
		trace.WithRegion(__jspt_ctx_10, "goroutine: anon", func() {
			{
				defer __jspt_wg_0.Done()
				trace.Log(__jspt_ctx_5,

					"spawn_child",
					fmt.
						Sprintf("sid=%d",

							__jspt_spawn_7))

				for currentPhase < phases {
					mu.Lock()
					for arrived < workers && currentPhase < phases {
						cond.Wait()
					}
					if currentPhase >= phases {
						mu.Unlock()
						return
					}
					currentPhase++
					arrived = 0
					cond.Broadcast()
					mu.Unlock()
				}
				close(updates)
			}
		})
	}(__jspt_ctx_5)

	// Workers: do phase work, send updates, then wait at barrier.
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
		go func(__jspt_ctx_11 context.Context,) {
			trace.WithRegion(__jspt_ctx_11, "goroutine: anon", func() {
				{
					defer __jspt_wg_0.Done()
					trace.Log(__jspt_ctx_5,

						"spawn_child",
						fmt.
							Sprintf("sid=%d",

								__jspt_spawn_8))

					defer wg.Done()
					localPhase := 0
					for localPhase < phases {
						time.Sleep(time.Duration(18+wid*7+localPhase*9) * time.Millisecond)

						select {
						case updates <- msg{from: wid, phase: localPhase}:
						case <-ctx.Done():
							return
						}

						// Barrier wait.
						mu.Lock()
						for currentPhase != localPhase && currentPhase < phases {
							cond.Wait()
						}
						arrived++
						cond.Signal()
						for currentPhase == localPhase && currentPhase < phases {
							cond.Wait()
						}
						mu.Unlock()

						localPhase++
					}
				}
			})
		}(__jspt_ctx_5)

	}
	__jspt_spawn_9 :=

	// Observer: reads updates concurrently.
	atomic.AddUint64(&__jspt_spawn_id_2, 1)
	trace.Log(__jspt_ctx_5,

		"spawn_parent",
		fmt.
			Sprintf("sid=%d",

				__jspt_spawn_9))
	__jspt_wg_0.Add(1)
	go func(__jspt_ctx_12 context.Context,) {
		trace.WithRegion(__jspt_ctx_12,

		// intentionally minimal
		"goroutine: anon", func() {
			{
				defer __jspt_wg_0.Done()
				trace.Log(__jspt_ctx_5,

					"spawn_child",
					fmt.
						Sprintf("sid=%d",

							__jspt_spawn_9))

				for range updates {

				}
			}
		})
	}(__jspt_ctx_5)

	wg.Wait()
	time.Sleep(40 * time.Millisecond)	// let coordinator/observer settle
	fmt.Println("done")
}
func init() {
	RegisterWorkload("crdntr",
		RuncrdntrProgram,
	)
}
