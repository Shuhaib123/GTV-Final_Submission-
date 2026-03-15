//go:build workload_crndtr1
// +build workload_crndtr1

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

type msg struct {
	from	int
	phase	int
}

func Runcrndtr1Program(__jspt_ctx_8 context.Context) {
	var __jspt_wg_0 sync.WaitGroup
	defer __jspt_wg_0.Wait()
	gtvtrace.InstallStopOnSignal()
	gtvtrace.InstallStopAfterFromEnv("GTV_TIMEOUT_MS",
	)
	__jspt_ctx_8, __jspt_task_9 := trace.NewTask(__jspt_ctx_8, "crndtr1")
	defer __jspt_task_9.End()
	trace.Log(__jspt_ctx_8, "main", "crndtr1 starting")

	ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
	defer cancel()

	const workers = 4
	const phases = 3

	updates := make(chan msg, 32)
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_8,

			"chan_make",
			fmt.Sprintf("ptr=%p cap=%d type=%s",

				updates,
				32, "msg"))
	}

	var mu sync.Mutex
	cond := sync.NewCond(&mu)
	currentPhase := 0
	arrived := 0
	__jspt_spawn_10 :=

	// Coordinator: advances phase when all workers arrive.
	atomic.AddUint64(&__jspt_spawn_id_2, 1)
	trace.Log(__jspt_ctx_8,

		"spawn_parent",
		fmt.
			Sprintf("sid=%d",

				__jspt_spawn_10),
	)
	__jspt_wg_0.Add(1)
	go func(__jspt_ctx_15 context.Context,) {
		trace.WithRegion(__jspt_ctx_15, "goroutine: anon", func() {
			{
				defer __jspt_wg_0.Done()
				trace.Log(__jspt_ctx_8,

					"spawn_child",
					fmt.
						Sprintf("sid=%d",

							__jspt_spawn_10))

				for currentPhase < phases {
					mu.Lock()
					for arrived < workers && currentPhase < phases {
						trace.WithRegion(__jspt_ctx_8, "cond.wait cond", func() {
							{
								cond.Wait()
							}
						})
					}
					if currentPhase >= phases {
						mu.Unlock()
						return
					}
					currentPhase++
					arrived = 0
					trace.WithRegion(__jspt_ctx_8, "cond.broadcast cond", func() {
						{
							cond.Broadcast()
						}
					})
					mu.Unlock()
				}
				trace.WithRegion(__jspt_ctx_8,

				// Workers: do phase work, send updates, then wait at barrier.
				"chan.close", func() {
					{
						if trace.IsEnabled() {
							trace.Log(__jspt_ctx_8,

								"chan_close",
								fmt.Sprintf("ptr=%p",

									updates,
								))
						}

						close(updates)
					}
				})
			}
		})
	}(__jspt_ctx_8)

	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		wid := w
		__jspt_spawn_11 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
		trace.Log(__jspt_ctx_8,

			"spawn_parent",
			fmt.
				Sprintf("sid=%d",

					__jspt_spawn_11),
		)
		__jspt_wg_0.Add(1)
		go func(__jspt_ctx_16 context.Context,) {
			trace.WithRegion(__jspt_ctx_16, "goroutine: anon", func() {
				{
					defer __jspt_wg_0.Done()
					trace.Log(__jspt_ctx_8,

						"spawn_child",
						fmt.
							Sprintf("sid=%d",

								__jspt_spawn_11))

					defer wg.Done()
					localPhase := 0
					for localPhase < phases {
						time.Sleep(time.Duration(18+wid*7+localPhase*9) * time.Millisecond)
						{
							__jspt_sel_12 := fmt.
								Sprintf(
									"s%v", time.
										Now().UnixNano())
							if trace.IsEnabled() {
								trace.Log(__jspt_ctx_8,

									"select", "select_begin id="+
										__jspt_sel_12,
								)
							}

							select {
							case __jspt_select_send_5(__jspt_ctx_8, "worker: send to "+"updates", updates) <- msg{from: wid, phase: localPhase}:
								if trace.IsEnabled() {
									trace.Log(__jspt_ctx_8,

										"select", "select_chosen id="+
											__jspt_sel_12,
									)
								}
								if trace.IsEnabled() {
									trace.Log(__jspt_ctx_8,

										"select_send",
										fmt.Sprintf("ptr=%p name=%s",

											updates,
											"worker: send to "+"updates"))
								}
								trace.WithRegion(__jspt_ctx_8, "worker: send to "+"updates", func() {
									{
									}
								})

							case <-__jspt_recv_in_region_4(__jspt_ctx_8,

							// Barrier wait.
							"worker: receive from "+"chan", ctx.Done()):
								if trace.IsEnabled() {
									trace.Log(__jspt_ctx_8,

										"select", "select_chosen id="+
											__jspt_sel_12,
									)
								}
								if trace.IsEnabled() {
									trace.Log(__jspt_ctx_8,

										"select_recv",
										fmt.Sprintf("ptr=%p name=%s",

											ctx.Done(), "worker: receive from "+"chan"))
								}

								return
							}
							if trace.IsEnabled() {
								trace.Log(__jspt_ctx_8,

									"select", "select_end id="+
										__jspt_sel_12,
								)
							}
						}

						mu.Lock()
						for currentPhase != localPhase && currentPhase < phases {
							trace.WithRegion(__jspt_ctx_8, "cond.wait cond", func() {
								{
									cond.Wait()
								}
							})
						}
						arrived++
						trace.WithRegion(__jspt_ctx_8, "cond.signal cond", func() {
							{
								cond.Signal()
							}
						})
						for currentPhase == localPhase && currentPhase < phases {
							trace.WithRegion(__jspt_ctx_8, "cond.wait cond", func() {
								{
									cond.Wait()
								}
							})
						}
						mu.Unlock()

						localPhase++
					}
				}
			})
		}(__jspt_ctx_8)

	}
	__jspt_spawn_13 :=

	// Observer: reads updates concurrently.
	atomic.AddUint64(&__jspt_spawn_id_2, 1)
	trace.Log(__jspt_ctx_8,

		"spawn_parent",
		fmt.
			Sprintf("sid=%d",

				__jspt_spawn_13),
	)
	__jspt_wg_0.Add(1)
	go func(__jspt_ctx_17 context.Context,) {
		trace.WithRegion(__jspt_ctx_17,

		// intentionally minimal
		"goroutine: anon", func() {
			{
				defer __jspt_wg_0.Done()
				trace.Log(__jspt_ctx_8,

					"spawn_child",
					fmt.
						Sprintf("sid=%d",

							__jspt_spawn_13))
				for {
					_, __jspt_ok_14 := __jspt_recv_value_ok_7(__jspt_ctx_8, "worker: receive from "+"updates", updates)
					if !__jspt_ok_14 {
						break
					}
				}
			}
		})
	}(__jspt_ctx_8)

	wg.Wait()
	time.Sleep(40 * time.Millisecond)	// let coordinator/observer settle
	fmt.Println("done")
}
func __jspt_recv_value_6[T any](
	__jspt_ctx_18 context.Context, label string, ch <-chan T) T {
	if trace.IsEnabled() {
		trace.
			Log(__jspt_ctx_18,

				"ch_ptr", fmt.Sprintf("ptr=%p", ch),
			)
		trace.Log(__jspt_ctx_18, "ch_name",

			fmt.Sprintf(
				"ptr=%p name=%s", ch, label,
			))
	}
	var v T
	trace.
		WithRegion(
			__jspt_ctx_18,
			label, func() { v = <-ch })
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_18, "value",
			fmt.Sprint(v))
	}
	return v
}
func __jspt_recv_value_ok_7[T any](__jspt_ctx_18 context.
	Context, label string, ch <-chan T) (T,
	bool) {
	if trace.
		IsEnabled() {
		trace.Log(__jspt_ctx_18,
			"ch_ptr",
			fmt.Sprintf("ptr=%p",

				ch))
		trace.Log(__jspt_ctx_18,

			"ch_name", fmt.Sprintf("ptr=%p name=%s", ch, label))
	}
	var v T
	var ok bool
	trace.WithRegion(__jspt_ctx_18, label, func() {
		v, ok = <-ch
	})
	if ok && trace.IsEnabled() {
		trace.Log(__jspt_ctx_18, "value", fmt.
			Sprint(v))
	}
	return v, ok
}
func __jspt_select_recv_3[T any](__jspt_ctx_19 context.Context, label string, ch <-chan T) <-chan T {
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_19,

			"ch_ptr",
			fmt.Sprintf("ptr=%p",

				ch))
		trace.Log(__jspt_ctx_19,

			"ch_name", fmt.
				Sprintf("ptr=%p name=%s", ch,
					label))
	}
	return ch
}
func __jspt_recv_in_region_4[T any](__jspt_ctx_19 context.Context, label string, ch <-chan T) <-chan T {
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_19, "ch_ptr", fmt.Sprintf("ptr=%p",
			ch))
		trace.Log(__jspt_ctx_19, "ch_name",
			fmt.Sprintf("ptr=%p name=%s",

				ch, label))
	}
	return ch
}
func __jspt_select_send_5[T any](
	__jspt_ctx_19 context.
		Context, label string, ch chan<- T) chan<- T {
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_19,
			"ch_ptr", fmt.
				Sprintf("ptr=%p", ch))
		trace.
			Log(__jspt_ctx_19,
				"ch_name",
				fmt.Sprintf(
					"ptr=%p name=%s", ch,
					label))
	}
	return ch
}
func init() {
	RegisterWorkload("crndtr1",
		Runcrndtr1Program,
	)
}
