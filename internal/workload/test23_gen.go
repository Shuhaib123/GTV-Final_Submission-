//go:build workload_test23
// +build workload_test23

// /workloads/strength_demo.go
package workload

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"
	"runtime/trace"
	"jspt/internal/gtvtrace"
	"sync/atomic"
)

var __jspt_spawn_id_2 uint64

type Job struct {
	ID	int
	Work	int
}

type Result struct {
	JobID	int
	Worker	int
	Latency	time.Duration
}

type Heartbeat struct {
	Tick	int
	From	string
}

func Runtest23Program(__jspt_ctx_8 context.Context) {
	var __jspt_wg_0 sync.WaitGroup
	defer __jspt_wg_0.Wait()
	gtvtrace.InstallStopOnSignal()
	gtvtrace.InstallStopAfterFromEnv("GTV_TIMEOUT_MS",
	)
	__jspt_ctx_8, __jspt_task_9 := trace.NewTask(__jspt_ctx_8, "test23")
	defer __jspt_task_9.End()
	trace.Log(__jspt_ctx_8, "main", "test23 starting")

	rand.Seed(time.Now().UnixNano())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Named channels (helps your UI label policy).
	jobs := make(chan Job)
	if trace.IsEnabled() {
		trace.
			Log(__jspt_ctx_8,

				"chan_make",
				fmt.Sprintf("ptr=%p cap=%d type=%s",

					jobs,
					0, "Job"))
	}

	// unbuffered => more visible block/unblock
	results := make(chan Result, 16)
	if trace.IsEnabled() {
		trace.
			Log(__jspt_ctx_8,

				"chan_make",
				fmt.Sprintf("ptr=%p cap=%d type=%s",

					results,
					16, "Result",
				))
	}

	// buffered => shows queueing and eventual drains
	cancelCh := make(chan struct{})
	if trace.IsEnabled() {
		trace.
			Log(__jspt_ctx_8,

				"chan_make",
				fmt.Sprintf("ptr=%p cap=%d type=%s",

					cancelCh,
					0, "struct{}",
				))
	}

	done := make(chan struct{})
	if trace.IsEnabled() {
		trace.
			Log(__jspt_ctx_8,

				"chan_make",
				fmt.Sprintf("ptr=%p cap=%d type=%s",

					done,
					0, "struct{}",
				))
	}

	// Broadcast demo channels.
	hbIn := make(chan Heartbeat)
	if trace.IsEnabled() {
		trace.
			Log(__jspt_ctx_8,

				"chan_make",
				fmt.Sprintf("ptr=%p cap=%d type=%s",

					hbIn,
					0, "Heartbeat",
				))
	}

	subA := make(chan Heartbeat, 4)
	if trace.IsEnabled() {
		trace.
			Log(__jspt_ctx_8,

				"chan_make",
				fmt.Sprintf("ptr=%p cap=%d type=%s",

					subA,
					4, "Heartbeat",
				))
	}

	subB := make(chan Heartbeat, 4)
	if trace.IsEnabled() {
		trace.
			Log(__jspt_ctx_8,

				"chan_make",
				fmt.Sprintf("ptr=%p cap=%d type=%s",

					subB,
					4, "Heartbeat",
				))
	}

	var wg sync.WaitGroup

	// Canceler: forces a clean stop (demonstrates cancellation edges).
	wg.Add(1)
	__jspt_spawn_10 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
	trace.Log(__jspt_ctx_8, "spawn_parent",
		fmt.
			Sprintf("sid=%d",
				__jspt_spawn_10,
			))
	__jspt_wg_0.Add(1)
	go func(__jspt_ctx_28 context.Context,) {
		trace.WithRegion(__jspt_ctx_28, "goroutine: anon", func() {
			{
				defer __jspt_wg_0.Done()
				trace.Log(__jspt_ctx_8, "spawn_child",
					fmt.Sprintf("sid=%d",
						__jspt_spawn_10,
					))

				defer wg.Done()
				time.Sleep(1800 * time.Millisecond)
				trace.WithRegion(__jspt_ctx_8, "chan.close",

				// Broadcaster: one-to-many (demonstrates broadcast topology).
				func() {
					{
						if trace.IsEnabled() {
							trace.
								Log(__jspt_ctx_8,

									"chan_close",
									fmt.Sprintf("ptr=%p",

										cancelCh,
									))
						}

						close(cancelCh)
					}
				})
				cancel()
			}
		})
	}(__jspt_ctx_8)

	wg.Add(1)
	__jspt_spawn_11 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
	trace.Log(__jspt_ctx_8, "spawn_parent",
		fmt.
			Sprintf("sid=%d",
				__jspt_spawn_11,
			))
	__jspt_wg_0.Add(1)
	go func(__jspt_ctx_29 context.Context,) {
		trace.WithRegion(__jspt_ctx_29, "goroutine: anon", func() {
			{
				defer __jspt_wg_0.Done()
				trace.Log(__jspt_ctx_8, "spawn_child",
					fmt.Sprintf("sid=%d",
						__jspt_spawn_11,
					))

				defer wg.Done()
				broadcaster(ctx, hbIn, []chan<- Heartbeat{subA, subB})
			}
		})
	}(__jspt_ctx_8)

	// Subscribers: show independent receives and occasional lag/backpressure.
	wg.Add(2)
	__jspt_spawn_12 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
	trace.Log(__jspt_ctx_8, "spawn_parent",
		fmt.
			Sprintf("sid=%d",
				__jspt_spawn_12,
			))
	__jspt_wg_0.Add(1)
	go func(__jspt_ctx_30 context.Context,) {
		trace.WithRegion(__jspt_ctx_30, "goroutine: anon", func() {
			{
				defer __jspt_wg_0.Done()
				trace.Log(__jspt_ctx_8, "spawn_child",
					fmt.Sprintf("sid=%d",
						__jspt_spawn_12,
					))

				defer wg.Done()
				subscriber(ctx, "subA", subA)
			}
		})
	}(__jspt_ctx_8)
	__jspt_spawn_13 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
	trace.Log(__jspt_ctx_8, "spawn_parent",
		fmt.
			Sprintf("sid=%d",
				__jspt_spawn_13,
			))
	__jspt_wg_0.Add(1)
	go func(__jspt_ctx_31 context.Context,) {
		trace.WithRegion(__jspt_ctx_31, "goroutine: anon", func() {

			// Heartbeat source: periodic sends into hbIn.
			{
				defer __jspt_wg_0.Done()
				trace.Log(__jspt_ctx_8, "spawn_child",
					fmt.Sprintf("sid=%d",
						__jspt_spawn_13,
					))

				defer wg.Done()
				subscriber(ctx, "subB", subB)
			}
		})
	}(__jspt_ctx_8)

	wg.Add(1)
	__jspt_spawn_14 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
	trace.Log(__jspt_ctx_8, "spawn_parent",
		fmt.
			Sprintf("sid=%d",
				__jspt_spawn_14,
			))
	__jspt_wg_0.Add(1)
	go func(__jspt_ctx_32 context.Context,) {
		trace.WithRegion(__jspt_ctx_32, "goroutine: anon", func() {
			{
				defer __jspt_wg_0.Done()
				trace.Log(__jspt_ctx_8, "spawn_child",
					fmt.Sprintf("sid=%d",
						__jspt_spawn_14,
					))

				defer wg.Done()
				t := time.NewTicker(120 * time.Millisecond)
				defer t.Stop()

				tick := 0
				for {
					{
						__jspt_sel_15 := fmt.Sprintf("s%v",
							time.Now().UnixNano())
						if trace.IsEnabled() {
							trace.
								Log(__jspt_ctx_8,

									"select",
									"select_begin id="+
										__jspt_sel_15,
								)
						}

						select {
						case <-__jspt_recv_in_region_4(__jspt_ctx_8, "worker: receive from "+"chan", ctx.Done()):
							if trace.IsEnabled() {
								trace.
									Log(__jspt_ctx_8,

										"select",
										"select_chosen id="+
											__jspt_sel_15,
									)
							}
							if trace.IsEnabled() {
								trace.
									Log(__jspt_ctx_8,

										"select_recv",
										fmt.
											Sprintf(
												"ptr=%p name=%s",

												ctx.Done(),
												"worker: receive from "+
													"chan"))
							}
							trace.WithRegion(__jspt_ctx_8, "chan.close", func() {
								{
									if trace.IsEnabled() {
										trace.
											Log(__jspt_ctx_8,

												"chan_close",
												fmt.Sprintf("ptr=%p",

													hbIn,
												))
									}

									close(hbIn)
								}
							})
							return
						case <-__jspt_recv_in_region_4(__jspt_ctx_8, "worker: receive from "+"c", t.C):
							if trace.IsEnabled() {
								trace.
									Log(__jspt_ctx_8,

										"select",
										"select_chosen id="+
											__jspt_sel_15,
									)
							}
							if trace.IsEnabled() {
								trace.
									Log(__jspt_ctx_8,

										"select_recv",
										fmt.
											Sprintf(
												"ptr=%p name=%s",

												t.C, "worker: receive from "+
													"c",
											))
							}
							trace.WithRegion(__jspt_ctx_8, "worker: receive from "+"c", func() {
								{

									tick++
									if trace.IsEnabled() {
										trace.
											Log(__jspt_ctx_8,

												"value",
												fmt.Sprint(Heartbeat{Tick: tick,

													From:	"heartbeat_source",
												}))
									}
									if trace.IsEnabled() {
										trace.
											Log(__jspt_ctx_8,

												"ch_ptr",
												fmt.Sprintf("ptr=%p",
													hbIn,
												),
											)
									}
									trace.WithRegion(__jspt_ctx_8, "worker: send to "+"hbin", func() {
										{

											hbIn <- Heartbeat{Tick: tick, From: "heartbeat_source"}
										}
									})
								}
							})
						}
						if trace.IsEnabled() {
							trace.
								Log(__jspt_ctx_8,

									"select",
									"select_end id="+
										__jspt_sel_15,
								)
						}
					}

				}
			}
		})
	}(__jspt_ctx_8)

	// Workers: fan-out processing from jobs -> results.
	workerCount := 3
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		__jspt_spawn_16 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
		trace.Log(__jspt_ctx_8, "spawn_parent",
			fmt.
				Sprintf("sid=%d",
					__jspt_spawn_16,
				))
		__jspt_wg_0.Add(1)
		go func(__jspt_ctx_33 context.Context, workerID int) {
			trace.WithRegion(__jspt_ctx_33, "goroutine: anon", func() {
				{
					defer __jspt_wg_0.

					// Producer: burst sends to trigger visible blocking on jobs.
					Done()
					trace.Log(__jspt_ctx_8, "spawn_child",
						fmt.Sprintf("sid=%d",
							__jspt_spawn_16,
						))

					defer wg.Done()
					worker(ctx, workerID, jobs, results)
				}
			})
		}(__jspt_ctx_8, i)
	}

	wg.Add(1)
	__jspt_spawn_17 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
	trace.Log(__jspt_ctx_8, "spawn_parent",
		fmt.
			Sprintf("sid=%d",
				__jspt_spawn_17,
			))
	__jspt_wg_0.Add(1)
	go func(__jspt_ctx_34 context.Context,) {
		trace.WithRegion(__jspt_ctx_34, "goroutine: anon", func() {
			{
				defer

				// Aggregator: fan-in results, uses select timeout to show select choices.
				__jspt_wg_0.Done()
				trace.Log(__jspt_ctx_8, "spawn_child",
					fmt.Sprintf("sid=%d",
						__jspt_spawn_17,
					))

				defer wg.Done()
				producer(ctx, jobs, cancelCh)
			}
		})
	}(__jspt_ctx_8)

	wg.Add(1)
	__jspt_spawn_18 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
	trace.Log(__jspt_ctx_8, "spawn_parent",
		fmt.
			Sprintf("sid=%d",
				__jspt_spawn_18,
			))
	__jspt_wg_0.Add(1)
	go func(__jspt_ctx_35 context.Context,) {
		trace.WithRegion(__jspt_ctx_35, "goroutine: anon", func() {
			{
				defer

				// Shutdown: wait for aggregator, then close results by stopping workers via ctx.
				__jspt_wg_0.Done()
				trace.Log(__jspt_ctx_8, "spawn_child",
					fmt.Sprintf("sid=%d",
						__jspt_spawn_18,
					))

				defer wg.Done()
				aggregator(ctx, results, done)
			}
		})
	}(__jspt_ctx_8)
	__jspt_recv_value_6(__jspt_ctx_8, "worker: receive from "+"done", done)

	cancel()
	wg.Wait()

	fmt.Println("done: strength_demo finished")
}

func producer(ctx context.Context, jobs chan<- Job, cancelCh <-chan struct{}) {
	defer close(jobs)

	for id := 1; id <= 30; id++ {
		work := 40 + rand.Intn(120)
		{
			__jspt_sel_19 := fmt.Sprintf("s%v",
				time.Now().UnixNano())
			if trace.IsEnabled() {
				trace.
					Log(ctx,
						"select",
						"select_begin id="+
							__jspt_sel_19,
					)
			}

			select {
			case <-__jspt_recv_in_region_4(ctx, "worker: receive from "+"chan", ctx.Done()):
				if trace.IsEnabled() {
					trace.
						Log(ctx,
							"select",
							"select_chosen id="+
								__jspt_sel_19,
						)
				}
				if trace.IsEnabled() {
					trace.
						Log(ctx,
							"select_recv",

							fmt.Sprintf("ptr=%p name=%s",

								ctx.
									Done(), "worker: receive from "+
									"chan",
							))
				}

				return
			case <-__jspt_recv_in_region_4(ctx, "worker: receive from "+"cancelch", cancelCh):
				if trace.IsEnabled() {
					trace.
						Log(ctx,
							"select",
							"select_chosen id="+
								__jspt_sel_19,
						)
				}
				if trace.IsEnabled() {
					trace.
						Log(ctx,
							"select_recv",

							fmt.Sprintf("ptr=%p name=%s",

								cancelCh,

								"worker: receive from "+
									"cancelch",
							))
				}

				return
			case __jspt_select_send_5(ctx, "worker: send to "+
			// Burst pattern: occasional extra sends to amplify blocking.
			"jobs", jobs) <- Job{ID: id, Work: work}:
				if trace.IsEnabled() {
					trace.
						Log(ctx,
							"select",
							"select_chosen id="+
								__jspt_sel_19,
						)
				}
				if trace.IsEnabled() {
					trace.
						Log(ctx,
							"select_send",

							fmt.Sprintf("ptr=%p name=%s",

								jobs,

								"worker: send to "+
									"jobs",
							))
				}
				trace.WithRegion(ctx, "worker: send to "+"jobs", func() {
					{

						if id%7 == 0 {
							time.Sleep(50 * time.Millisecond)
						}
					}
				})
			}
			if trace.IsEnabled() {
				trace.
					Log(ctx,
						"select",
						"select_end id="+
							__jspt_sel_19,
					)
			}
		}

	}
}

func worker(ctx context.Context, workerID int, jobs <-chan Job, results chan<- Result) {
	for {
		{
			__jspt_sel_20 := fmt.Sprintf("s%v",
				time.Now().UnixNano())
			if trace.IsEnabled() {
				trace.
					Log(ctx,
						"select",
						"select_begin id="+
							__jspt_sel_20,
					)
			}

			select {
			case <-__jspt_recv_in_region_4(ctx, "worker: receive from "+"chan", ctx.Done()):
				if trace.IsEnabled() {
					trace.
						Log(ctx,
							"select",
							"select_chosen id="+
								__jspt_sel_20,
						)
				}
				if trace.IsEnabled() {
					trace.
						Log(ctx,
							"select_recv",

							fmt.Sprintf("ptr=%p name=%s",

								ctx.
									Done(), "worker: receive from "+
									"chan",
							))
				}

				return
			case job, ok := <-__jspt_recv_in_region_4(ctx, "worker: receive from "+"jobs", jobs):
				if trace.IsEnabled() {
					trace.
						Log(ctx,
							"select",
							"select_chosen id="+
								__jspt_sel_20,
						)
				}
				if trace.IsEnabled() {
					trace.
						Log(ctx,
							"select_recv",

							fmt.Sprintf("ptr=%p name=%s",

								jobs,

								"worker: receive from "+
									"jobs"))
				}

				if !ok {
					return
				}

				start := time.Now()

				// One slow worker creates backpressure => blocks/unblocks become obvious.
				if workerID == 0 {
					time.Sleep(time.Duration(job.Work+140) * time.Millisecond)
				} else {
					time.Sleep(time.Duration(job.Work) * time.Millisecond)
				}

				res := Result{
					JobID:		job.ID,
					Worker:		workerID,
					Latency:	time.Since(start),
				}
				{
					__jspt_sel_21 := fmt.Sprintf("s%v",
						time.Now().UnixNano())
					if trace.IsEnabled() {
						trace.
							Log(ctx,
								"select",
								"select_begin id="+
									__jspt_sel_21,
							)
					}

					select {
					case <-__jspt_recv_in_region_4(ctx, "worker: receive from "+"chan", ctx.Done()):
						if trace.IsEnabled() {
							trace.
								Log(ctx,
									"select",
									"select_chosen id="+
										__jspt_sel_21,
								)
						}
						if trace.IsEnabled() {
							trace.
								Log(ctx,
									"select_recv",

									fmt.Sprintf("ptr=%p name=%s",

										ctx.
											Done(), "worker: receive from "+
											"chan",
									))
						}

						return
					case __jspt_select_send_5(ctx, "worker: send to "+"results", results) <- res:
						if trace.IsEnabled() {
							trace.
								Log(ctx,
									"select",
									"select_chosen id="+
										__jspt_sel_21,
								)
						}
						if trace.IsEnabled() {
							trace.
								Log(ctx,
									"select_send",

									fmt.Sprintf("ptr=%p name=%s",

										results,

										"worker: send to "+
											"results"),
								)
						}
						trace.WithRegion(ctx, "worker: send to "+"results", func() {
							{
							}
						})

					}
					if trace.IsEnabled() {
						trace.
							Log(ctx,
								"select",
								"select_end id="+
									__jspt_sel_21,
							)
					}
				}

			}
			if trace.IsEnabled() {
				trace.
					Log(ctx,
						"select",
						"select_end id="+
							__jspt_sel_20,
					)
			}
		}

	}
}

func aggregator(ctx context.Context, results <-chan Result, done chan<- struct{}) {
	defer close(done)

	seen := 0
	timeout := time.NewTimer(300 * time.Millisecond)
	defer timeout.Stop()

	for {
		timeout.Reset(300 * time.Millisecond)
		{
			__jspt_sel_22 := fmt.Sprintf("s%v",
				time.Now().UnixNano())
			if trace.IsEnabled() {
				trace.
					Log(ctx,
						"select",
						"select_begin id="+
							__jspt_sel_22,
					)
			}

			select {
			case <-__jspt_recv_in_region_4(ctx, "worker: receive from "+"chan", ctx.Done()):
				if trace.IsEnabled() {
					trace.
						Log(ctx,
							"select",
							"select_chosen id="+
								__jspt_sel_22,
						)
				}
				if trace.IsEnabled() {
					trace.
						Log(ctx,
							"select_recv",

							fmt.Sprintf("ptr=%p name=%s",

								ctx.
									Done(), "worker: receive from "+
									"chan",
							))
				}

				return
			case __jspt_recv_23 := <-__jspt_recv_in_region_4(ctx, "worker: receive from "+"results", results):
				if trace.IsEnabled() {
					trace.
						Log(ctx,
							"select",
							"select_chosen id="+
								__jspt_sel_22,
						)
				}
				if trace.IsEnabled() {
					trace.
						Log(ctx,
							"select_recv",

							fmt.Sprintf("ptr=%p name=%s",

								results,

								"worker: receive from "+
									"results",
							))
				}

				r := __jspt_recv_23
				seen++
				fmt.Printf("result: job=%02d worker=%d latency=%v\n", r.JobID, r.Worker, r.Latency)
				if seen >= 20 {
					return
				}
			case <-__jspt_recv_in_region_4(
			// This is deliberate: demonstrates select choice without data available.
			ctx, "worker: receive from "+"c", timeout.C):
				if trace.IsEnabled() {
					trace.
						Log(ctx,
							"select",
							"select_chosen id="+
								__jspt_sel_22,
						)
				}
				if trace.IsEnabled() {
					trace.
						Log(ctx,
							"select_recv",

							fmt.Sprintf("ptr=%p name=%s",

								timeout.
									C, "worker: receive from "+
									"c",
							))

				}
				trace.WithRegion(ctx, "worker: receive from "+"c", func() {
					{

						fmt.Println("aggregator: timeout (no result ready)")
					}
				})
			}
			if trace.IsEnabled() {
				trace.
					Log(ctx,
						"select",
						"select_end id="+
							__jspt_sel_22,
					)
			}
		}

	}
}

func broadcaster(ctx context.Context, in <-chan Heartbeat, outs []chan<- Heartbeat) {
	for {
		{
			__jspt_sel_24 := fmt.Sprintf("s%v",
				time.Now().UnixNano())
			if trace.IsEnabled() {
				trace.
					Log(ctx,
						"select",
						"select_begin id="+
							__jspt_sel_24,
					)
			}

			select {
			case <-__jspt_recv_in_region_4(ctx, "worker: receive from "+

			// leave open; subscribers exit via ctx
			"chan", ctx.Done()):
				if trace.IsEnabled() {
					trace.
						Log(ctx,
							"select",
							"select_chosen id="+
								__jspt_sel_24,
						)
				}
				if trace.IsEnabled() {
					trace.
						Log(ctx,
							"select_recv",

							fmt.Sprintf("ptr=%p name=%s",

								ctx.
									Done(), "worker: receive from "+
									"chan",
							))
				}

				for _, ch := range outs {

					_ = ch
				}
				return
			case hb, ok := <-__jspt_recv_in_region_4(ctx, "worker: receive from "+

			// Broadcast fan-out: sequential sends show per-subscriber backpressure.
			"in", in):
				if trace.IsEnabled() {
					trace.
						Log(ctx,
							"select",
							"select_chosen id="+
								__jspt_sel_24,
						)
				}
				if trace.IsEnabled() {
					trace.
						Log(ctx,
							"select_recv",

							fmt.Sprintf("ptr=%p name=%s",

								in,

								"worker: receive from "+
									"in"))
				}

				if !ok {
					return
				}

				for _, out := range outs {
					{
						__jspt_sel_25 := fmt.Sprintf("s%v",
							time.Now().UnixNano())
						if trace.IsEnabled() {
							trace.
								Log(ctx,
									"select",
									"select_begin id="+
										__jspt_sel_25,
								)
						}

						select {
						case <-__jspt_recv_in_region_4(ctx, "worker: receive from "+"chan", ctx.Done()):
							if trace.IsEnabled() {
								trace.
									Log(ctx,
										"select",
										"select_chosen id="+
											__jspt_sel_25,
									)
							}
							if trace.IsEnabled() {
								trace.
									Log(ctx,
										"select_recv",

										fmt.Sprintf("ptr=%p name=%s",

											ctx.
												Done(), "worker: receive from "+
												"chan",
										))
							}

							return
						case __jspt_select_send_5(ctx, "worker: send to "+"out", out) <- hb:
							if trace.IsEnabled() {
								trace.
									Log(ctx,
										"select",
										"select_chosen id="+
											__jspt_sel_25,
									)
							}
							if trace.IsEnabled() {
								trace.
									Log(ctx,
										"select_send",

										fmt.Sprintf("ptr=%p name=%s",

											out,

											"worker: send to "+
												"out",
										))
							}
							trace.WithRegion(ctx, "worker: send to "+"out", func() {
								{
								}
							})

						}
						if trace.IsEnabled() {
							trace.
								Log(ctx,
									"select",
									"select_end id="+
										__jspt_sel_25,
								)
						}
					}

				}
			}
			if trace.IsEnabled() {
				trace.
					Log(ctx,
						"select",
						"select_end id="+
							__jspt_sel_24,
					)
			}
		}

	}
}

func subscriber(ctx context.Context, name string, in <-chan Heartbeat) {
	for {
		{
			__jspt_sel_26 := fmt.Sprintf("s%v",
				time.Now().UnixNano())
			if trace.IsEnabled() {
				trace.
					Log(ctx,
						"select",
						"select_begin id="+
							__jspt_sel_26,
					)
			}

			select {
			case <-__jspt_recv_in_region_4(ctx, "worker: receive from "+

			// Occasional lag to produce visible blocking in broadcaster.
			"chan", ctx.Done()):
				if trace.IsEnabled() {
					trace.
						Log(ctx,
							"select",
							"select_chosen id="+
								__jspt_sel_26,
						)
				}
				if trace.IsEnabled() {
					trace.
						Log(ctx,
							"select_recv",

							fmt.Sprintf("ptr=%p name=%s",

								ctx.
									Done(), "worker: receive from "+
									"chan",
							))
				}

				return
			case __jspt_recv_27 := <-__jspt_recv_in_region_4(ctx, "worker: receive from "+"in", in):
				if trace.IsEnabled() {
					trace.
						Log(ctx,
							"select",
							"select_chosen id="+
								__jspt_sel_26,
						)
				}
				if trace.IsEnabled() {
					trace.
						Log(ctx,
							"select_recv",

							fmt.Sprintf("ptr=%p name=%s",

								in,

								"worker: receive from "+
									"in"))
				}
				trace.WithRegion(ctx, "worker: receive from "+"in", func() {
					{

						hb := __jspt_recv_27

						if hb.Tick%5 == 0 && name == "subB" {
							time.Sleep(180 * time.Millisecond)
						}
						_ = hb
						if trace.IsEnabled() {
							trace.
								Log(ctx,
									"value",
									fmt.Sprint(__jspt_recv_27,
									))
						}
					}
				})

			}
			if trace.IsEnabled() {
				trace.
					Log(ctx,
						"select",
						"select_end id="+
							__jspt_sel_26,
					)
			}
		}

	}
}
func __jspt_recv_value_6[T any](
	__jspt_ctx_36 context.
		Context, label string, ch <-chan T) T {
	if trace.IsEnabled() {
		trace.
			Log(__jspt_ctx_36,
				"ch_ptr",

				fmt.Sprintf("ptr=%p",
					ch))
		trace.Log(
			__jspt_ctx_36,

			"ch_name",
			fmt.Sprintf("ptr=%p name=%s",

				ch, label))
	}
	var v T
	trace.WithRegion(__jspt_ctx_36,
		label, func() {
			v = <-ch
		})
	if trace.IsEnabled() {
		trace.
			Log(__jspt_ctx_36, "value", fmt.Sprint(v))
	}
	return v
}
func __jspt_recv_value_ok_7[T any](__jspt_ctx_36 context.Context, label string, ch <-chan T) (T, bool) {
	if trace.IsEnabled() {
		trace.
			Log(__jspt_ctx_36, "ch_ptr",

				fmt.Sprintf("ptr=%p", ch))
		trace.Log(__jspt_ctx_36, "ch_name",
			fmt.Sprintf("ptr=%p name=%s",
				ch, label))
	}
	var v T
	var ok bool
	trace.WithRegion(__jspt_ctx_36, label, func() { v, ok = <-ch })
	if ok && trace.IsEnabled() {
		trace.Log(
			__jspt_ctx_36, "value",
			fmt.Sprint(v))
	}
	return v, ok
}
func __jspt_select_recv_3[T any](
	__jspt_ctx_37 context.
		Context, label string, ch <-chan T) <-chan T {
	if trace.
		IsEnabled() {
		trace.Log(__jspt_ctx_37,

			"ch_ptr", fmt.Sprintf("ptr=%p",
				ch))
		trace.
			Log(__jspt_ctx_37,
				"ch_name", fmt.Sprintf("ptr=%p name=%s",

					ch, label))
	}
	return ch
}
func __jspt_recv_in_region_4[T any](__jspt_ctx_37 context.Context, label string, ch <-chan T) <-chan T {
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_37, "ch_ptr",
			fmt.Sprintf("ptr=%p", ch))
		trace.Log(__jspt_ctx_37, "ch_name", fmt.Sprintf("ptr=%p name=%s",
			ch, label))
	}
	return ch
}
func __jspt_select_send_5[T any](__jspt_ctx_37 context.Context, label string, ch chan<- T) chan<- T {
	if trace.
		IsEnabled() {
		trace.Log(__jspt_ctx_37,

			"ch_ptr", fmt.
				Sprintf("ptr=%p", ch))
		trace.Log(__jspt_ctx_37, "ch_name", fmt.Sprintf("ptr=%p name=%s",
			ch,
			label))
	}
	return ch
}
func init() {
	RegisterWorkload("test23", Runtest23Program,
	)
}
