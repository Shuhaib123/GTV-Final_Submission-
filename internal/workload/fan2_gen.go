//go:build workload_fan2
// +build workload_fan2

package workload

import (
	"context"
	"fmt"
	"jspt/internal/gtvtrace"
	"runtime/trace"
	"sync"
	"sync/atomic"
	"time"
)

var __jspt_spawn_id_2 uint64

type item struct {
	id  int
	val int
}

func Runfan2Program(__jspt_ctx_8 context.Context) {
	var __jspt_wg_0 sync.WaitGroup
	defer __jspt_wg_0.Wait()
	gtvtrace.InstallStopOnSignal()
	gtvtrace.InstallStopAfterFromEnv("GTV_TIMEOUT_MS")
	__jspt_ctx_8, __jspt_task_9 := trace.NewTask(__jspt_ctx_8, "fan2")
	defer __jspt_task_9.End()
	trace.Log(__jspt_ctx_8, "main", "fan2 starting")

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
	func(__jspt_ctx_19 context.Context) {
		defer __jspt_wg_0.Done()
		trace.Log(__jspt_ctx_8,

			"spawn_child",
			fmt.
				Sprintf("sid=%d",

					__jspt_spawn_10))

		defer close(in)
		for i := 0; i < 20; i++ {
			{
				__jspt_sel_11 := fmt.
					Sprintf(
						"s%v", time.
							Now().UnixNano())
				if trace.IsEnabled() {
					trace.Log(__jspt_ctx_8,

						"select", "select_begin id="+
							__jspt_sel_11,
					)
				}

				select {
				case <-__jspt_recv_in_region_4(__jspt_ctx_8, "worker: receive from "+"chan", ctx.Done()):
					if trace.IsEnabled() {
						trace.Log(__jspt_ctx_8,

							"select", "select_chosen id="+
								__jspt_sel_11,
						)
					}
					if trace.IsEnabled() {
						trace.Log(__jspt_ctx_8,

							"select_recv",
							fmt.Sprintf("ptr=%p name=%s",

								ctx.
									Done(), "worker: receive from "+"chan"))
					}

					return
				case __jspt_select_send_5(__jspt_ctx_8, "worker: send to "+"in", in) <- item{id: i, val: i * 2}:
					if trace.IsEnabled() {
						trace.Log(__jspt_ctx_8,

							"select", "select_chosen id="+
								__jspt_sel_11,
						)
					}
					if trace.IsEnabled() {
						trace.Log(__jspt_ctx_8,

							"select_send",
							fmt.Sprintf("ptr=%p name=%s",

								in,
								"worker: send to "+"in"))
					}
					trace.WithRegion(__jspt_ctx_8, "worker: send to "+"in", func() {
						{

							time.Sleep(8 * time.Millisecond)
						}
					})
				}
				if trace.IsEnabled() {
					trace.Log(__jspt_ctx_8,

						"select", "select_end id="+
							__jspt_sel_11,
					)
				}
			}

		}
	}(__jspt_ctx_8)

	// Stage 2: fan-out workers
	const workers = 3
	var wg sync.WaitGroup
	wg.Add(workers)

	for w := 0; w < workers; w++ {
		wid := w
		__jspt_spawn_12 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
		trace.Log(__jspt_ctx_8,

			"spawn_parent",
			fmt.
				Sprintf("sid=%d",

					__jspt_spawn_12))
		__jspt_wg_0.Add(1)
		go func(__jspt_ctx_20 context.Context) {
			defer __jspt_wg_0.Done()
			trace.Log(__jspt_ctx_8,

				"spawn_child",
				fmt.
					Sprintf("sid=%d",

						__jspt_spawn_12))

			defer wg.Done()
			for it := range in {
				{
					__jspt_sel_13 := fmt.
						Sprintf(
							"s%v", time.
								Now().UnixNano())
					if trace.IsEnabled() {
						trace.Log(__jspt_ctx_8,

							"select", "select_begin id="+
								__jspt_sel_13,
						)
					}

					select {
					case <-__jspt_recv_in_region_4(__jspt_ctx_8, "worker: receive from "+"chan", ctx.Done()):
						if trace.IsEnabled() {
							trace.Log(__jspt_ctx_8,

								"select", "select_chosen id="+
									__jspt_sel_13,
							)
						}
						if trace.IsEnabled() {
							trace.Log(__jspt_ctx_8,

								"select_recv",
								fmt.Sprintf("ptr=%p name=%s",

									ctx.
										Done(), "worker: receive from "+"chan"))
						}

						return
					case <-time.After(time.Duration(10+it.id%7) * time.Millisecond):
						it.val += wid
					}
					if trace.IsEnabled() {
						trace.Log(__jspt_ctx_8,

							"select", "select_end id="+
								__jspt_sel_13,
						)
					}
				}
				{
					__jspt_sel_14 := fmt.
						Sprintf(
							"s%v", time.
								Now().UnixNano())
					if trace.IsEnabled() {
						trace.Log(__jspt_ctx_8,

							"select", "select_begin id="+
								__jspt_sel_14,
						)
					}

					select {
					case <-__jspt_recv_in_region_4(__jspt_ctx_8, "worker: receive from "+"chan", ctx.Done()):
						if trace.IsEnabled() {
							trace.Log(__jspt_ctx_8,

								"select", "select_chosen id="+
									__jspt_sel_14,
							)
						}
						if trace.IsEnabled() {
							trace.Log(__jspt_ctx_8,

								"select_recv",
								fmt.Sprintf("ptr=%p name=%s",

									ctx.
										Done(), "worker: receive from "+"chan"))
						}

						return
					case __jspt_select_send_5(__jspt_ctx_8, "worker: send to "+

						// Close out when workers finish
						"out", out) <- it:
						if trace.IsEnabled() {
							trace.Log(__jspt_ctx_8,

								"select", "select_chosen id="+
									__jspt_sel_14,
							)
						}
						if trace.IsEnabled() {
							trace.Log(__jspt_ctx_8,

								"select_send",
								fmt.Sprintf("ptr=%p name=%s",

									out,
									"worker: send to "+"out"))
						}
						trace.WithRegion(__jspt_ctx_8, "worker: send to "+"out", func() {
							{
							}
						})

					}
					if trace.IsEnabled() {
						trace.Log(__jspt_ctx_8,

							"select", "select_end id="+
								__jspt_sel_14,
						)
					}
				}

			}
		}(__jspt_ctx_8)

	}
	__jspt_spawn_15 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
	trace.Log(__jspt_ctx_8,

		"spawn_parent",
		fmt.
			Sprintf("sid=%d",

				__jspt_spawn_15))
	__jspt_wg_0.Add(1)
	go func(__jspt_ctx_21 context.Context) {
		defer __jspt_wg_0.Done()
		trace.Log(__jspt_ctx_8,

			"spawn_child",
			fmt.
				Sprintf("sid=%d",

					__jspt_spawn_15))

		wg.Wait()
		trace.WithRegion(__jspt_ctx_8,

			// Stage 3: aggregator
			"chan.close", func() {
				{
					if trace.IsEnabled() {
						trace.Log(__jspt_ctx_8,

							"chan_close",
							fmt.Sprintf("ptr=%p",

								out))
					}

					close(out)
				}
			})
	}(__jspt_ctx_8)
	__jspt_spawn_16 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
	trace.Log(__jspt_ctx_8,

		"spawn_parent",
		fmt.
			Sprintf("sid=%d",

				__jspt_spawn_16))
	__jspt_wg_0.Add(1)
	go func(__jspt_ctx_22 context.Context) {
		defer __jspt_wg_0.Done()
		trace.Log(__jspt_ctx_8,

			"spawn_child",
			fmt.
				Sprintf("sid=%d",

					__jspt_spawn_16))

		sum := 0
		for {
			{
				__jspt_sel_17 := fmt.
					Sprintf(
						"s%v", time.
							Now().UnixNano())
				if trace.IsEnabled() {
					trace.Log(__jspt_ctx_8,

						"select", "select_begin id="+
							__jspt_sel_17,
					)
				}

				select {
				case <-__jspt_recv_in_region_4(__jspt_ctx_8, "worker: receive from "+"chan", ctx.Done()):
					if trace.IsEnabled() {
						trace.Log(__jspt_ctx_8,

							"select", "select_chosen id="+
								__jspt_sel_17,
						)
					}
					if trace.IsEnabled() {
						trace.Log(__jspt_ctx_8,

							"select_recv",
							fmt.Sprintf("ptr=%p name=%s",

								ctx.
									Done(), "worker: receive from "+"chan"))
					}
					if trace.IsEnabled() {
						trace.Log(__jspt_ctx_8,

							"ch_ptr", fmt.
								Sprintf(
									"ptr=%p",
									done,
								))
					}
					trace.WithRegion(__jspt_ctx_8, "worker: send to "+"done", func() {
						{

							done <- sum
						}
					})
					return
				case __jspt_recv_18 := <-__jspt_recv_in_region_4(__jspt_ctx_8, "worker: receive from "+"out", out):
					if trace.IsEnabled() {
						trace.Log(__jspt_ctx_8,

							"select", "select_chosen id="+
								__jspt_sel_17,
						)
					}
					if trace.IsEnabled() {
						trace.Log(__jspt_ctx_8,

							"select_recv",
							fmt.Sprintf("ptr=%p name=%s",

								out,
								"worker: receive from "+"out"))
					}

					it, ok := __jspt_recv_18
					if !ok {
						if trace.IsEnabled() {
							trace.Log(__jspt_ctx_8,

								"ch_ptr", fmt.
									Sprintf(
										"ptr=%p",
										done,
									))
						}
						trace.WithRegion(__jspt_ctx_8, "worker: send to "+"done", func() {
							{

								done <- sum
							}
						})
						return
					}
					sum += it.val
				}
				if trace.IsEnabled() {
					trace.Log(__jspt_ctx_8,

						"select", "select_end id="+
							__jspt_sel_17,
					)
				}
			}

		}
	}(__jspt_ctx_8)

	fmt.Println("sum =", <-done)
}
func __jspt_select_recv_3[
	T any](__jspt_ctx_23 context.Context, label string, ch <-chan T) <-chan T {
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_23,

			"ch_ptr",
			fmt.Sprintf("ptr=%p", ch))
		trace.Log(__jspt_ctx_23,

			"ch_name", fmt.Sprintf("ptr=%p name=%s",
				ch, label))
	}
	return ch
}
func __jspt_recv_in_region_4[T any](__jspt_ctx_23 context.
	Context, label string, ch <-chan T) <-chan T {
	if trace.IsEnabled() {
		trace.
			Log(__jspt_ctx_23,

				"ch_ptr", fmt.Sprintf("ptr=%p", ch))
		trace.Log(__jspt_ctx_23,
			"ch_name", fmt.
				Sprintf("ptr=%p name=%s", ch, label))
	}
	return ch
}
func __jspt_select_send_5[T any](__jspt_ctx_23 context.Context, label string, ch chan<- T) chan<- T {
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_23, "ch_ptr",
			fmt.Sprintf("ptr=%p",
				ch,
			),
		)
		trace.Log(__jspt_ctx_23, "ch_name",
			fmt.Sprintf("ptr=%p name=%s",
				ch,
				label,
			))
	}
	return ch
}
func init() {
	RegisterWorkload("fan2",
		Runfan2Program,
	)
}
