//go:build workload_richsample
// +build workload_richsample

package workload

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
	"runtime/trace"
	"jspt/internal/gtvtrace"
)

var __jspt_spawn_id_2 uint64

func RunrichsampleProgram(
// Keep it short but event-rich.
__jspt_ctx_8 context.Context) {
	var __jspt_wg_0 sync.WaitGroup
	defer __jspt_wg_0.Wait()
	gtvtrace.InstallStopOnSignal()
	gtvtrace.InstallStopAfterFromEnv("GTV_TIMEOUT_MS",
	)
	__jspt_ctx_8, __jspt_task_9 := trace.NewTask(__jspt_ctx_8, "richsample")
	defer __jspt_task_9.End()
	trace.Log(__jspt_ctx_8, "main", "richsample starting")

	ctx, cancel := context.WithTimeout(context.Background(), 900*time.Millisecond)
	defer cancel()

	var wg sync.WaitGroup
	trace.

	// -------- 1) Spawn edges + goroutine lifecycles --------
	WithRegion(__jspt_ctx_8, "wg.add wg", func() {
		{

			wg.Add(1)
		}
	})
	__jspt_spawn_10 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
	trace.Log(__jspt_ctx_8,

		"spawn_parent",
		fmt.
			Sprintf("sid=%d", __jspt_spawn_10,
			))
	__jspt_wg_0.Add(1)
	go func(__jspt_ctx_45 context.Context,) {
		trace.WithRegion(__jspt_ctx_45, "goroutine: anon",

		// -------- 2) Buffered channel: cap/len, buffer_full/empty, blocking/unblocking --------
		func() {
			{
				defer __jspt_wg_0.Done()
				trace.Log(__jspt_ctx_8,

					"spawn_child",
					fmt.
						Sprintf("sid=%d", __jspt_spawn_10,
						))

				defer wg.Done()
				workerPool(ctx)
			}
		})
	}(__jspt_ctx_8)
	trace.WithRegion(__jspt_ctx_8, "wg.add wg", func() {
		{

			wg.Add(1)
		}
	})
	__jspt_spawn_11 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
	trace.Log(__jspt_ctx_8,

		"spawn_parent",
		fmt.
			Sprintf("sid=%d", __jspt_spawn_11,
			))
	__jspt_wg_0.Add(1)
	go func(__jspt_ctx_46 context.Context,) {
		trace.WithRegion(__jspt_ctx_46, "goroutine: anon", func() {

			// -------- 3) Unbuffered channel: rendezvous blocking/unblocking --------
			{
				defer __jspt_wg_0.Done()
				trace.Log(__jspt_ctx_8,

					"spawn_child",
					fmt.
						Sprintf("sid=%d", __jspt_spawn_11,
						))

				defer wg.Done()
				bufferedChannelDemo(ctx)
			}
		})
	}(__jspt_ctx_8)
	trace.WithRegion(__jspt_ctx_8, "wg.add wg", func() {
		{

			wg.Add(1)
		}
	})
	__jspt_spawn_12 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
	trace.Log(__jspt_ctx_8,

		"spawn_parent",
		fmt.
			Sprintf("sid=%d", __jspt_spawn_12,
			))
	__jspt_wg_0.Add(1)
	go func(__jspt_ctx_47 context.Context,) {
		trace.WithRegion(__jspt_ctx_47, "goroutine: anon", func() {

			// -------- 4) Select semantics: candidates + chosen + default + timeout --------
			{
				defer __jspt_wg_0.Done()
				trace.Log(__jspt_ctx_8,

					"spawn_child",
					fmt.
						Sprintf("sid=%d", __jspt_spawn_12,
						))

				defer wg.Done()
				unbufferedRendezvous(ctx)
			}
		})
	}(__jspt_ctx_8)
	trace.WithRegion(__jspt_ctx_8, "wg.add wg", func() {
		{

			wg.Add(1)
		}
	})
	__jspt_spawn_13 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
	trace.Log(__jspt_ctx_8,

		"spawn_parent",
		fmt.
			Sprintf("sid=%d", __jspt_spawn_13,
			))
	__jspt_wg_0.Add(1)
	go func(__jspt_ctx_48 context.Context,) {
		trace.WithRegion(__jspt_ctx_48, "goroutine: anon",

		// -------- 5) Mutex + RWMutex contention --------
		func() {
			{
				defer __jspt_wg_0.Done()
				trace.Log(__jspt_ctx_8,

					"spawn_child",
					fmt.
						Sprintf("sid=%d", __jspt_spawn_13,
						))

				defer wg.Done()
				selectDemo(ctx)
			}
		})
	}(__jspt_ctx_8)
	trace.WithRegion(__jspt_ctx_8, "wg.add wg", func() {
		{

			wg.Add(1)
		}
	})
	__jspt_spawn_14 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
	trace.Log(__jspt_ctx_8,

		"spawn_parent",
		fmt.
			Sprintf("sid=%d", __jspt_spawn_14,
			))
	__jspt_wg_0.Add(1)
	go func(__jspt_ctx_49 context.Context,) {
		trace.WithRegion(__jspt_ctx_49, "goroutine: anon", func

		// -------- 6) Cond wait/signal/broadcast (classic teaching pattern) --------
		() {
			{
				defer __jspt_wg_0.Done()
				trace.Log(__jspt_ctx_8,

					"spawn_child",
					fmt.
						Sprintf("sid=%d", __jspt_spawn_14,
						))

				defer wg.Done()
				mutexContention(ctx)
			}
		})
	}(__jspt_ctx_8)
	trace.WithRegion(__jspt_ctx_8, "wg.add wg", func() {
		{

			wg.Add(1)
		}
	})
	__jspt_spawn_15 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
	trace.Log(__jspt_ctx_8,

		"spawn_parent",
		fmt.
			Sprintf("sid=%d", __jspt_spawn_15,
			))
	__jspt_wg_0.Add(1)
	go func(__jspt_ctx_50 context.Context,) {
		trace.WithRegion(__jspt_ctx_50, "goroutine: anon", func() {
			{
				defer __jspt_wg_0.Done()
				trace.Log(__jspt_ctx_8,

					"spawn_child",
					fmt.
						Sprintf("sid=%d", __jspt_spawn_15,
						))

				defer wg.Done()
				condDemo(ctx)
			}
		})
	}(__jspt_ctx_8)
	trace.WithRegion(__jspt_ctx_8, "wg.wait wg", func() {
		{

			wg.Wait()
		}
	})
	fmt.Println("done")
}

func workerPool(ctx context.Context) {
	jobs := make(chan int, 3)
	if trace.IsEnabled() {
		trace.Log(ctx, "chan_make",
			fmt.
				Sprintf("ptr=%p cap=%d type=%s",
					jobs, 3, "int"))
	}

	// small buffer: can fill and block senders if receiver stalls
	results := make(chan int)
	if trace.IsEnabled() {
		trace.Log(ctx, "chan_make",
			fmt.
				Sprintf("ptr=%p cap=%d type=%s",
					results, 0, "int",
				))
	}

	// unbuffered: forces rendezvous

	var counter int64
	var wg sync.WaitGroup

	// Start a few workers (spawn edges + goroutine states)
	for i := 0; i < 3; i++ {
		trace.WithRegion(ctx, "wg.add wg", func() {
			{
				wg.Add(1)
			}
		})
		__jspt_spawn_16 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
		trace.Log(ctx,

			"spawn_parent",
			fmt.
				Sprintf("sid=%d",
					__jspt_spawn_16,
				))
		go func(__jspt_ctx_51 context.Context, id int) {
			trace.WithRegion(__jspt_ctx_51, "goroutine: anon", func() {
				{
					trace.Log(ctx,

						"spawn_child",
						fmt.
							Sprintf("sid=%d",
								__jspt_spawn_16,
							))

					defer wg.Done()
					for {
						{
							__jspt_sel_17 := fmt.
								Sprintf(
									"s%v", time.Now().UnixNano())
							if trace.IsEnabled() {
								trace.Log(ctx, "select",
									"select_begin id="+
										__jspt_sel_17,
								)
							}

							select {
							case <-__jspt_recv_in_region_4(ctx, "worker: receive from "+"chan", ctx.Done()):
								if trace.IsEnabled() {
									trace.Log(ctx, "select",
										fmt.Sprintf("select_chosen id=%s idx=0",
											__jspt_sel_17))
								}
								if trace.IsEnabled() {
									trace.Log(ctx, "select",
										fmt.Sprintf("select_case id=%s idx=0 kind=recv ptr=%p name=%q",

											__jspt_sel_17, ctx.Done(), "worker: receive from "+"chan"))
								}
								if trace.IsEnabled() {
									trace.Log(ctx, "select_recv",

										fmt.
											Sprintf("ptr=%p name=%q",
												ctx.Done(), "worker: receive from "+
													"chan"))
								}

								return
							case j, ok := <-__jspt_recv_in_region_4(ctx, "worker: receive from "+"jobs", jobs):
								if trace.IsEnabled() {
									trace.Log(ctx, "select",
										fmt.Sprintf("select_chosen id=%s idx=1",
											__jspt_sel_17))
								}
								if trace.IsEnabled() {
									trace.Log(ctx, "select",
										fmt.Sprintf("select_case id=%s idx=1 kind=recv ptr=%p name=%q",

											__jspt_sel_17, jobs, "worker: receive from "+"jobs"))
								}
								if trace.IsEnabled() {
									trace.Log(ctx, "select_recv",

										fmt.
											Sprintf("ptr=%p name=%q",
												jobs, "worker: receive from "+
													"jobs"))
								}

								if !ok {
									return
								}
								atomic.AddInt64(&counter, 1)

								// Simulate work and occasional blocking behavior.
								if j%2 == 0 {
									time.Sleep(20 * time.Millisecond)
								} else {
									time.Sleep(5 * time.Millisecond)
								}
								{
									__jspt_sel_18 := fmt.
										Sprintf(
											"s%v", time.Now().UnixNano())
									if trace.IsEnabled() {
										trace.Log(ctx, "select",
											"select_begin id="+
												__jspt_sel_18,
										)
									}

									// Send result: may block if receiver isn't ready.
									select {
									case __jspt_select_send_5(ctx, "worker: send to "+"results", results) <- (j * 2):
										if trace.IsEnabled() {
											trace.Log(ctx, "select",
												fmt.Sprintf("select_chosen id=%s idx=0",
													__jspt_sel_18))
										}
										if trace.IsEnabled() {
											trace.Log(ctx, "select",
												fmt.Sprintf("select_case id=%s idx=0 kind=send ptr=%p name=%q",

													__jspt_sel_18, results, "worker: send to "+"results"))
										}
										if trace.IsEnabled() {
											trace.Log(ctx, "select_send",

												fmt.
													Sprintf("ptr=%p name=%q",
														results, "worker: send to "+
															"results"))
										}
										trace.WithRegion(ctx, "worker: send to "+"results", func() {
											{
											}
										})

									case <-__jspt_recv_in_region_4(ctx, "worker: receive from "+"chan", ctx.Done()):
										if trace.IsEnabled() {
											trace.Log(ctx, "select",
												fmt.Sprintf("select_chosen id=%s idx=1",
													__jspt_sel_18))
										}
										if trace.IsEnabled() {
											trace.Log(ctx, "select",
												fmt.Sprintf("select_case id=%s idx=1 kind=recv ptr=%p name=%q",

													__jspt_sel_18, ctx.Done(), "worker: receive from "+"chan"))
										}
										if trace.IsEnabled() {
											trace.Log(ctx, "select_recv",

												fmt.
													Sprintf("ptr=%p name=%q",
														ctx.Done(), "worker: receive from "+
															"chan"))
										}

										return
									}
									if trace.IsEnabled() {
										trace.Log(ctx, "select",
											"select_end id="+
												__jspt_sel_18,
										)
									}
								}

							}
							if trace.IsEnabled() {
								trace.Log(ctx, "select",
									"select_end id="+
										__jspt_sel_17,
								)
							}
						}

					}
				}
			})
		}(ctx, i)
	}
	__jspt_spawn_19 :=

	// Producer: may block when jobs buffer is full.
	atomic.AddUint64(&__jspt_spawn_id_2, 1)
	trace.Log(ctx,

		"spawn_parent",
		fmt.
			Sprintf("sid=%d",
				__jspt_spawn_19,
			))
	go func(__jspt_ctx_52 context.Context,) {
		trace.WithRegion(__jspt_ctx_52, "goroutine: anon", func() {
			{
				trace.Log(ctx,

					"spawn_child",
					fmt.
						Sprintf("sid=%d",
							__jspt_spawn_19,
						))

				defer close(jobs)
				for j := 0; j < 10; j++ {
					{
						__jspt_sel_20 := fmt.
							Sprintf(
								"s%v", time.Now().UnixNano())
						if trace.IsEnabled() {
							trace.Log(ctx, "select",
								"select_begin id="+
									__jspt_sel_20,
							)
						}

						select {
						case __jspt_select_send_5(ctx, "worker: send to "+"jobs", jobs) <- j:
							if trace.IsEnabled() {
								trace.Log(ctx, "select",
									fmt.Sprintf("select_chosen id=%s idx=0",
										__jspt_sel_20))
							}
							if trace.IsEnabled() {
								trace.Log(ctx, "select",
									fmt.Sprintf("select_case id=%s idx=0 kind=send ptr=%p name=%q",

										__jspt_sel_20, jobs, "worker: send to "+"jobs"))
							}
							if trace.IsEnabled() {
								trace.Log(ctx, "select_send",

									fmt.
										Sprintf("ptr=%p name=%q",
											jobs, "worker: send to "+
												"jobs"))
							}
							trace.WithRegion(ctx, "worker: send to "+"jobs", func() {
								{
								}
							})

						case <-__jspt_recv_in_region_4(ctx, "worker: receive from "+"chan", ctx.Done()):
							if trace.IsEnabled() {
								trace.Log(ctx, "select",
									fmt.Sprintf("select_chosen id=%s idx=1",
										__jspt_sel_20))
							}
							if trace.IsEnabled() {
								trace.Log(ctx, "select",
									fmt.Sprintf("select_case id=%s idx=1 kind=recv ptr=%p name=%q",

										__jspt_sel_20, ctx.Done(), "worker: receive from "+"chan"))
							}
							if trace.IsEnabled() {
								trace.Log(ctx, "select_recv",

									fmt.
										Sprintf("ptr=%p name=%q",
											ctx.Done(), "worker: receive from "+
												"chan"))
							}

							return
						}
						if trace.IsEnabled() {
							trace.Log(ctx, "select",
								"select_end id="+
									__jspt_sel_20,
							)
						}
					}

					time.Sleep(7 * time.Millisecond)
				}
			}
		})
	}(ctx)

	// Collector: sometimes intentionally slow to create backpressure.
	__jspt_spawn_21 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
	trace.Log(ctx,

		"spawn_parent",
		fmt.
			Sprintf("sid=%d",
				__jspt_spawn_21,
			))
	go func(__jspt_ctx_53 context.Context,) {
		trace.WithRegion(__jspt_ctx_53, "goroutine: anon", func() {
			{
				trace.Log(ctx,

					"spawn_child",
					fmt.
						Sprintf("sid=%d",
							__jspt_spawn_21,
						))

				for {
					{
						__jspt_sel_22 := fmt.
							Sprintf(
								"s%v", time.Now().UnixNano())
						if trace.IsEnabled() {
							trace.Log(ctx, "select",
								"select_begin id="+
									__jspt_sel_22,
							)
						}

						select {
						case <-__jspt_recv_in_region_4(ctx, "worker: receive from "+"chan", ctx.Done()):
							if trace.IsEnabled() {
								trace.Log(ctx, "select",
									fmt.Sprintf("select_chosen id=%s idx=0",
										__jspt_sel_22))
							}
							if trace.IsEnabled() {
								trace.Log(ctx, "select",
									fmt.Sprintf("select_case id=%s idx=0 kind=recv ptr=%p name=%q",

										__jspt_sel_22, ctx.Done(), "worker: receive from "+"chan"))
							}
							if trace.IsEnabled() {
								trace.Log(ctx, "select_recv",

									fmt.
										Sprintf("ptr=%p name=%q",
											ctx.Done(), "worker: receive from "+
												"chan"))
							}

							return
						case __jspt_recv_23 := <-__jspt_recv_in_region_4(ctx, "worker: receive from "+"results", results):
							if trace.IsEnabled() {
								trace.Log(ctx, "select",
									fmt.Sprintf("select_chosen id=%s idx=1",
										__jspt_sel_22))
							}
							if trace.IsEnabled() {
								trace.Log(ctx, "select",
									fmt.Sprintf("select_case id=%s idx=1 kind=recv ptr=%p name=%q",

										__jspt_sel_22, results, "worker: receive from "+"results"))
							}
							if trace.IsEnabled() {
								trace.Log(ctx, "select_recv",

									fmt.
										Sprintf("ptr=%p name=%q",
											results, "worker: receive from "+
												"results"))
							}
							trace.WithRegion(ctx, "worker: receive from "+"results", func() {
								{

									r := __jspt_recv_23
									_ = r
									time.Sleep(10 * time.Millisecond)
									if trace.IsEnabled() {
										trace.Log(ctx, "value",
											fmt.Sprint(__jspt_recv_23,
											))
									}
								}
							})

						}
						if trace.IsEnabled() {
							trace.Log(ctx, "select",
								"select_end id="+
									__jspt_sel_22,
							)
						}
					}

				}
			}
		})
	}(ctx)
	trace.WithRegion(ctx, "wg.wait wg", func() {
		{

			wg.Wait()
		}
	})
	_ = atomic.LoadInt64(&counter)
}

func bufferedChannelDemo(ctx context.Context) {
	ch := make(chan int, 2)
	if trace.IsEnabled() {
		trace.Log(ctx, "chan_make",
			fmt.
				Sprintf("ptr=%p cap=%d type=%s",
					ch, 2, "int"))
	}

	// cap=2 -> easy to hit buffer_full
	var wg sync.WaitGroup
	trace.

	// Receiver is delayed: sender will fill buffer and then block on 3rd send.
	WithRegion(ctx, "wg.add wg", func() {
		{

			wg.Add(1)
		}
	})
	__jspt_spawn_24 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
	trace.Log(ctx,

		"spawn_parent",
		fmt.
			Sprintf("sid=%d",
				__jspt_spawn_24,
			))
	go func(__jspt_ctx_54 context.Context,) {
		trace.WithRegion(__jspt_ctx_54, "goroutine: anon", func() {
			{
				trace.Log(ctx,

					"spawn_child",
					fmt.
						Sprintf("sid=%d",
							__jspt_spawn_24,
						))

				defer wg.Done()
				time.Sleep(80 * time.Millisecond)
				for {
					{
						__jspt_sel_25 := fmt.
							Sprintf(
								"s%v", time.Now().UnixNano())
						if trace.IsEnabled() {
							trace.Log(ctx, "select",
								"select_begin id="+
									__jspt_sel_25,
							)
						}

						select {
						case <-__jspt_recv_in_region_4(ctx, "worker: receive from "+"chan", ctx.Done()):
							if trace.IsEnabled() {
								trace.Log(ctx, "select",
									fmt.Sprintf("select_chosen id=%s idx=0",
										__jspt_sel_25))
							}
							if trace.IsEnabled() {
								trace.Log(ctx, "select",
									fmt.Sprintf("select_case id=%s idx=0 kind=recv ptr=%p name=%q",

										__jspt_sel_25, ctx.Done(), "worker: receive from "+"chan"))
							}
							if trace.IsEnabled() {
								trace.Log(ctx, "select_recv",

									fmt.
										Sprintf("ptr=%p name=%q",
											ctx.Done(), "worker: receive from "+
												"chan"))
							}

							return
						case __jspt_recv_26 := <-__jspt_recv_in_region_4(ctx, "worker: receive from "+

						// keep it slower than sender
						"ch", ch):
							if trace.IsEnabled() {
								trace.Log(ctx, "select",
									fmt.Sprintf("select_chosen id=%s idx=1",
										__jspt_sel_25))
							}
							if trace.IsEnabled() {
								trace.Log(ctx, "select",
									fmt.Sprintf("select_case id=%s idx=1 kind=recv ptr=%p name=%q",

										__jspt_sel_25, ch, "worker: receive from "+"ch"))
							}
							if trace.IsEnabled() {
								trace.Log(ctx, "select_recv",

									fmt.
										Sprintf("ptr=%p name=%q",
											ch, "worker: receive from "+
												"ch"))
							}
							trace.WithRegion(ctx, "worker: receive from "+"ch", func() {
								{

									v := __jspt_recv_26
									_ = v
									time.Sleep(25 * time.Millisecond)
									if trace.IsEnabled() {
										trace.Log(ctx, "value",
											fmt.Sprint(__jspt_recv_26,
											))
									}
								}
							})

						}
						if trace.IsEnabled() {
							trace.Log(ctx, "select",
								"select_end id="+
									__jspt_sel_25,
							)
						}
					}

				}
			}
		})
	}(ctx)

	// Sender: will block when buffer is full.
	trace.WithRegion(ctx, "wg.add wg", func() {
		{

			wg.Add(1)
		}
	})
	__jspt_spawn_27 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
	trace.Log(ctx,

		"spawn_parent",
		fmt.
			Sprintf("sid=%d",
				__jspt_spawn_27,
			))
	go func(__jspt_ctx_55 context.Context,) {
		trace.WithRegion(__jspt_ctx_55, "goroutine: anon", func() {
			{
				trace.Log(ctx,

					"spawn_child",
					fmt.
						Sprintf("sid=%d",
							__jspt_spawn_27,
						))

				defer wg.Done()
				for i := 0; i < 6; i++ {
					{
						__jspt_sel_28 := fmt.
							Sprintf(
								"s%v", time.Now().UnixNano())
						if trace.IsEnabled() {
							trace.Log(ctx, "select",
								"select_begin id="+
									__jspt_sel_28,
							)
						}

						select {
						case <-__jspt_recv_in_region_4(ctx, "worker: receive from "+

						// keep pushing to force buffer-full blocking
						"chan", ctx.Done()):
							if trace.IsEnabled() {
								trace.Log(ctx, "select",
									fmt.Sprintf("select_chosen id=%s idx=0",
										__jspt_sel_28))
							}
							if trace.IsEnabled() {
								trace.Log(ctx, "select",
									fmt.Sprintf("select_case id=%s idx=0 kind=recv ptr=%p name=%q",

										__jspt_sel_28, ctx.Done(), "worker: receive from "+"chan"))
							}
							if trace.IsEnabled() {
								trace.Log(ctx, "select_recv",

									fmt.
										Sprintf("ptr=%p name=%q",
											ctx.Done(), "worker: receive from "+
												"chan"))
							}

							return
						case __jspt_select_send_5(ctx, "worker: send to "+"ch", ch) <- i:
							if trace.IsEnabled() {
								trace.Log(ctx, "select",
									fmt.Sprintf("select_chosen id=%s idx=1",
										__jspt_sel_28))
							}
							if trace.IsEnabled() {
								trace.Log(ctx, "select",
									fmt.Sprintf("select_case id=%s idx=1 kind=send ptr=%p name=%q",

										__jspt_sel_28, ch, "worker: send to "+"ch"))
							}
							if trace.IsEnabled() {
								trace.Log(ctx, "select_send",

									fmt.
										Sprintf("ptr=%p name=%q",
											ch, "worker: send to "+
												"ch"))
							}
							trace.WithRegion(ctx, "worker: send to "+"ch", func() {
								{
								}
							})

						}
						if trace.IsEnabled() {
							trace.Log(ctx, "select",
								"select_end id="+
									__jspt_sel_28,
							)
						}
					}

					time.Sleep(10 * time.Millisecond)
				}
			}
		})
	}(ctx)
	trace.WithRegion(ctx, "wg.wait wg", func() {
		{

			wg.Wait()
		}
	})
}

func unbufferedRendezvous(ctx context.Context) {
	ch := make(chan string)
	if trace.IsEnabled() {
		trace.Log(ctx, "chan_make",
			fmt.
				Sprintf("ptr=%p cap=%d type=%s",
					ch, 0, "string"),
		)
	}

	// unbuffered rendezvous

	var wg sync.WaitGroup
	trace.WithRegion(

	// Sender: blocks until receiver ready.
	ctx, "wg.add wg", func() {
		{
			wg.Add(2)
		}
	})
	__jspt_spawn_29 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
	trace.Log(ctx,

		"spawn_parent",
		fmt.
			Sprintf("sid=%d",
				__jspt_spawn_29,
			))
	go func(__jspt_ctx_56 context.Context,) {
		trace.WithRegion(__jspt_ctx_56, "goroutine: anon", func() {
			{
				trace.Log(ctx,

					"spawn_child",
					fmt.
						Sprintf("sid=%d",
							__jspt_spawn_29,
						))

				defer wg.Done()
				for i := 0; i < 5; i++ {
					{
						__jspt_sel_30 := fmt.
							Sprintf(
								"s%v", time.Now().UnixNano())
						if trace.IsEnabled() {
							trace.Log(ctx, "select",
								"select_begin id="+
									__jspt_sel_30,
							)
						}

						select {
						case <-__jspt_recv_in_region_4(ctx, "worker: receive from "+"chan", ctx.Done()):
							if trace.IsEnabled() {
								trace.Log(ctx, "select",
									fmt.Sprintf("select_chosen id=%s idx=0",
										__jspt_sel_30))
							}
							if trace.IsEnabled() {
								trace.Log(ctx, "select",
									fmt.Sprintf("select_case id=%s idx=0 kind=recv ptr=%p name=%q",

										__jspt_sel_30, ctx.Done(), "worker: receive from "+"chan"))
							}
							if trace.IsEnabled() {
								trace.Log(ctx, "select_recv",

									fmt.
										Sprintf("ptr=%p name=%q",
											ctx.Done(), "worker: receive from "+
												"chan"))
							}

							return
						case __jspt_select_send_5(ctx, "worker: send to "+
						// send succeeded
						"ch", ch) <- fmt.Sprintf("msg-%d", i):
							if trace.IsEnabled() {
								trace.Log(ctx, "select",
									fmt.Sprintf("select_chosen id=%s idx=1",
										__jspt_sel_30))
							}
							if trace.IsEnabled() {
								trace.Log(ctx, "select",
									fmt.Sprintf("select_case id=%s idx=1 kind=send ptr=%p name=%q",

										__jspt_sel_30, ch, "worker: send to "+"ch"))
							}
							if trace.IsEnabled() {
								trace.Log(ctx, "select_send",

									fmt.
										Sprintf("ptr=%p name=%q",
											ch, "worker: send to "+
												"ch"))
							}
							trace.WithRegion(ctx, "worker: send to "+"ch", func() {
								{
								}
							})

						}
						if trace.IsEnabled() {
							trace.Log(ctx, "select",
								"select_end id="+
									__jspt_sel_30,
							)
						}
					}

					time.Sleep(15 * time.Millisecond)
				}
			}
		})
	}(ctx)

	// Receiver: occasionally delays to create sender blocking.
	__jspt_spawn_31 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
	trace.Log(ctx,

		"spawn_parent",
		fmt.
			Sprintf("sid=%d",
				__jspt_spawn_31,
			))
	go func(__jspt_ctx_57 context.Context,) {
		trace.WithRegion(__jspt_ctx_57, "goroutine: anon", func() {
			{
				trace.Log(ctx,

					"spawn_child",
					fmt.
						Sprintf("sid=%d",
							__jspt_spawn_31,
						))

				defer wg.Done()
				for {
					{
						__jspt_sel_32 := fmt.
							Sprintf(
								"s%v", time.Now().UnixNano())
						if trace.IsEnabled() {
							trace.Log(ctx, "select",
								"select_begin id="+
									__jspt_sel_32,
							)
						}

						select {
						case <-__jspt_recv_in_region_4(ctx, "worker: receive from "+"chan", ctx.Done()):
							if trace.IsEnabled() {
								trace.Log(ctx, "select",
									fmt.Sprintf("select_chosen id=%s idx=0",
										__jspt_sel_32))
							}
							if trace.IsEnabled() {
								trace.Log(ctx, "select",
									fmt.Sprintf("select_case id=%s idx=0 kind=recv ptr=%p name=%q",

										__jspt_sel_32, ctx.Done(), "worker: receive from "+"chan"))
							}
							if trace.IsEnabled() {
								trace.Log(ctx, "select_recv",

									fmt.
										Sprintf("ptr=%p name=%q",
											ctx.Done(), "worker: receive from "+
												"chan"))
							}

							return
						case __jspt_recv_33 := <-__jspt_recv_in_region_4(ctx, "worker: receive from "+"ch", ch):
							if trace.IsEnabled() {
								trace.Log(ctx, "select",
									fmt.Sprintf("select_chosen id=%s idx=1",
										__jspt_sel_32))
							}
							if trace.IsEnabled() {
								trace.Log(ctx, "select",
									fmt.Sprintf("select_case id=%s idx=1 kind=recv ptr=%p name=%q",

										__jspt_sel_32, ch, "worker: receive from "+"ch"))
							}
							if trace.IsEnabled() {
								trace.Log(ctx, "select_recv",

									fmt.
										Sprintf("ptr=%p name=%q",
											ch, "worker: receive from "+
												"ch"))
							}
							trace.WithRegion(ctx, "worker: receive from "+"ch", func() {
								{

									m := __jspt_recv_33
									_ = m
									time.Sleep(30 * time.Millisecond)
									if trace.IsEnabled() {
										trace.Log(ctx, "value",
											fmt.Sprint(__jspt_recv_33,
											))
									}
								}
							})

						}
						if trace.IsEnabled() {
							trace.Log(ctx, "select",
								"select_end id="+
									__jspt_sel_32,
							)
						}
					}

				}
			}
		})
	}(ctx)
	trace.WithRegion(ctx, "wg.wait wg", func() {
		{

			wg.Wait()
		}
	})
}

func selectDemo(ctx context.Context) {
	a := make(chan int, 1)
	if trace.IsEnabled() {
		trace.Log(ctx, "chan_make",
			fmt.
				Sprintf("ptr=%p cap=%d type=%s",
					a, 1, "int"))
	}

	b := make(chan int)
	if trace.IsEnabled() {
		trace.Log(ctx, "chan_make",
			fmt.
				Sprintf("ptr=%p cap=%d type=%s",
					b, 0, "int"))
	}

	// unbuffered
	done := make(chan struct{})
	if trace.IsEnabled() {
		trace.Log(ctx, "chan_make",
			fmt.
				Sprintf("ptr=%p cap=%d type=%s",
					done, 0, "struct{}",
				))
	}
	__jspt_spawn_34 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
	trace.Log(ctx,

		"spawn_parent",
		fmt.
			Sprintf("sid=%d",
				__jspt_spawn_34,
			))
	go

	// Producer for 'a' (buffered): makes case occasionally ready
	func(__jspt_ctx_58 context.Context,) {
		trace.WithRegion(__jspt_ctx_58, "goroutine: anon", func() {
			{
				trace.Log(ctx,

					"spawn_child",
					fmt.
						Sprintf("sid=%d",
							__jspt_spawn_34,
						))

				defer close(done)
				for i := 0; i < 3; i++ {
					if trace.IsEnabled() {
						trace.Log(ctx, "value",
							fmt.Sprint(i))
					}
					if trace.IsEnabled() {
						trace.Log(ctx, "ch_ptr",
							fmt.Sprintf("ptr=%p",
								a))
					}
					trace.WithRegion(ctx, "worker: send to "+"a", func() {
						{

							a <- i
						}
					})
					time.Sleep(35 * time.Millisecond)
				}
			}
		})
	}(ctx)

	// Receiver for 'b' starts late so sending to b blocks at first
	__jspt_spawn_35 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
	trace.Log(ctx,

		"spawn_parent",
		fmt.
			Sprintf("sid=%d",
				__jspt_spawn_35,
			))
	go func(__jspt_ctx_59 context.Context,) {
		trace.WithRegion(__jspt_ctx_59, "goroutine: anon", func() {
			{
				trace.Log(ctx,

					"spawn_child",
					fmt.
						Sprintf("sid=%d",
							__jspt_spawn_35,
						))

				time.Sleep(120 * time.Millisecond)
				for {
					{
						__jspt_sel_36 := fmt.
							Sprintf(
								"s%v", time.Now().UnixNano())
						if trace.IsEnabled() {
							trace.Log(ctx, "select",
								"select_begin id="+
									__jspt_sel_36,
							)
						}

						select {
						case <-__jspt_recv_in_region_4(ctx, "worker: receive from "+"chan", ctx.Done()):
							if trace.IsEnabled() {
								trace.Log(ctx, "select",
									fmt.Sprintf("select_chosen id=%s idx=0",
										__jspt_sel_36))
							}
							if trace.IsEnabled() {
								trace.Log(ctx, "select",
									fmt.Sprintf("select_case id=%s idx=0 kind=recv ptr=%p name=%q",

										__jspt_sel_36, ctx.Done(), "worker: receive from "+"chan"))
							}
							if trace.IsEnabled() {
								trace.Log(ctx, "select_recv",

									fmt.
										Sprintf("ptr=%p name=%q",
											ctx.Done(), "worker: receive from "+
												"chan"))
							}

							return
						case <-__jspt_recv_in_region_4(ctx, "worker: receive from "+"b", b):
							if trace.IsEnabled() {
								trace.Log(ctx, "select",
									fmt.Sprintf("select_chosen id=%s idx=1",
										__jspt_sel_36))
							}
							if trace.IsEnabled() {
								trace.Log(ctx, "select",
									fmt.Sprintf("select_case id=%s idx=1 kind=recv ptr=%p name=%q",

										__jspt_sel_36, b, "worker: receive from "+"b"))
							}
							if trace.IsEnabled() {
								trace.Log(ctx, "select_recv",

									fmt.
										Sprintf("ptr=%p name=%q",
											b, "worker: receive from "+
												"b"))
							}
							trace.WithRegion(ctx, "worker: receive from "+"b", func() {
								{
								}
							})

						}
						if trace.IsEnabled() {
							trace.Log(ctx, "select",
								"select_end id="+
									__jspt_sel_36,
							)
						}
					}

				}
			}
		})
	}(ctx)

	for {
		select {
		case <-__jspt_recv_in_region_4(ctx, "worker: receive from "+"chan", ctx.Done()):
			if trace.IsEnabled() {
				trace.Log(ctx, "select_recv",

					fmt.
						Sprintf("ptr=%p name=%q",
							ctx.Done(), "worker: receive from "+
								"chan"))
			}

			return
		case <-__jspt_recv_in_region_4(ctx, "worker: receive from "+

		// default case chosen when nothing else is ready: non-blocking
		"done", done):
			if trace.IsEnabled() {
				trace.Log(ctx, "select_recv",

					fmt.
						Sprintf("ptr=%p name=%q",
							done, "worker: receive from "+
								"done"))
			}

			return
		default:

			time.Sleep(8 * time.Millisecond)
		}
		{
			__jspt_sel_37 := fmt.
				Sprintf(
					"s%v", time.Now().UnixNano())
			if trace.IsEnabled() {
				trace.Log(ctx, "select",
					"select_begin id="+
						__jspt_sel_37,
				)
			}

			// A second select that can block (timeout candidate)
			select {
			case <-__jspt_recv_in_region_4(ctx, "worker: receive from "+

			// chosen when 'a' ready
			"chan", ctx.Done()):
				if trace.IsEnabled() {
					trace.Log(ctx, "select",
						fmt.Sprintf("select_chosen id=%s idx=0",
							__jspt_sel_37))
				}
				if trace.IsEnabled() {
					trace.Log(ctx, "select",
						fmt.Sprintf("select_case id=%s idx=0 kind=recv ptr=%p name=%q",

							__jspt_sel_37, ctx.Done(), "worker: receive from "+"chan"))
				}
				if trace.IsEnabled() {
					trace.Log(ctx, "select_recv",

						fmt.
							Sprintf("ptr=%p name=%q",
								ctx.Done(), "worker: receive from "+
									"chan"))
				}

				return
			case __jspt_recv_38 := <-__jspt_recv_in_region_4(ctx, "worker: receive from "+

			// chosen once receiver is alive
			"a", a):
				if trace.IsEnabled() {
					trace.Log(ctx, "select",
						fmt.Sprintf("select_chosen id=%s idx=1",
							__jspt_sel_37))
				}
				if trace.IsEnabled() {
					trace.Log(ctx, "select",
						fmt.Sprintf("select_case id=%s idx=1 kind=recv ptr=%p name=%q",

							__jspt_sel_37, a, "worker: receive from "+"a"))
				}
				if trace.IsEnabled() {
					trace.Log(ctx, "select_recv",

						fmt.
							Sprintf("ptr=%p name=%q",
								a, "worker: receive from "+
									"a"))
				}
				trace.WithRegion(ctx, "worker: receive from "+"a", func() {
					{

						x := __jspt_recv_38
						_ = x
						if trace.IsEnabled() {
							trace.Log(ctx, "value",
								fmt.Sprint(__jspt_recv_38,
								))
						}
					}
				})

			case __jspt_select_send_5(ctx, "worker: send to "+"b", b) <- 42:
				if trace.IsEnabled() {
					trace.Log(ctx, "select",
						fmt.Sprintf("select_chosen id=%s idx=2",
							__jspt_sel_37))
				}
				if trace.IsEnabled() {
					trace.Log(ctx, "select",
						fmt.Sprintf("select_case id=%s idx=2 kind=send ptr=%p name=%q",

							__jspt_sel_37, b, "worker: send to "+"b"))
				}
				if trace.IsEnabled() {
					trace.Log(ctx, "select_send",

						fmt.
							Sprintf("ptr=%p name=%q",
								b, "worker: send to "+
									"b"))
				}
				trace.WithRegion(ctx, "worker: send to "+"b", func() {
					{
					}
				})

			case <-time.After(18 * time.Millisecond):
				// chosen on timeout: teaches time-based select
			}
			if trace.IsEnabled() {
				trace.Log(ctx, "select",
					"select_end id="+
						__jspt_sel_37,
				)
			}
		}

	}
}

func mutexContention(ctx context.Context) {
	var mu sync.Mutex
	var rw sync.RWMutex
	var shared int

	var wg sync.WaitGroup
	trace.WithRegion(

	// Mutex writer (holds lock long enough to create contention)
	ctx, "wg.add wg", func() {
		{
			wg.Add(3)
		}
	})
	__jspt_spawn_39 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
	trace.Log(ctx,

		"spawn_parent",
		fmt.
			Sprintf("sid=%d",
				__jspt_spawn_39,
			))
	go func(__jspt_ctx_60 context.Context,) {
		trace.WithRegion(__jspt_ctx_60, "goroutine: anon", func() {
			{
				trace.Log(ctx,

					"spawn_child",
					fmt.
						Sprintf("sid=%d",
							__jspt_spawn_39,
						))

				defer wg.Done()
				for i := 0; i < 6; i++ {
					select {
					case <-__jspt_recv_in_region_4(ctx, "worker: receive from "+"chan", ctx.Done()):
						if trace.IsEnabled() {
							trace.Log(ctx, "select_recv",

								fmt.
									Sprintf("ptr=%p name=%q",
										ctx.Done(), "worker: receive from "+
											"chan"))
						}

						return
					default:
						time.
							Sleep(1 *
								time.Millisecond,
							)

					}
					trace.WithRegion(ctx, "mutex.lock mu", func() {
						{
							mu.Lock()
						}
					})
					shared++
					time.Sleep(20 * time.Millisecond)
					trace.WithRegion(ctx, "mutex.unlock mu", func() {
						{
							mu.Unlock()
						}
					})
					time.Sleep(10 * time.Millisecond)
				}
			}
		})
	}(ctx)

	// RWMutex readers (contend with a writer below)
	__jspt_spawn_40 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
	trace.Log(ctx,

		"spawn_parent",
		fmt.
			Sprintf("sid=%d",
				__jspt_spawn_40,
			))
	go func(__jspt_ctx_61 context.Context,) {
		trace.WithRegion(__jspt_ctx_61, "goroutine: anon", func() {
			{
				trace.Log(ctx,

					"spawn_child",
					fmt.
						Sprintf("sid=%d",
							__jspt_spawn_40,
						))

				defer wg.Done()
				for {
					select {
					case <-__jspt_recv_in_region_4(ctx, "worker: receive from "+"chan", ctx.Done()):
						if trace.IsEnabled() {
							trace.Log(ctx, "select_recv",

								fmt.
									Sprintf("ptr=%p name=%q",
										ctx.Done(), "worker: receive from "+
											"chan"))
						}

						return
					default:
						time.
							Sleep(1 *
								time.Millisecond,
							)

					}
					trace.WithRegion(ctx, "rwmutex.rlock rw", func() {
						{
							rw.RLock()
						}
					})
					_ = shared
					time.Sleep(12 * time.Millisecond)
					trace.WithRegion(ctx, "rwmutex.runlock rw", func() {
						{
							rw.RUnlock()
						}
					})
					time.Sleep(6 * time.Millisecond)
				}
			}
		})
	}(ctx)

	// RWMutex writer
	__jspt_spawn_41 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
	trace.Log(ctx,

		"spawn_parent",
		fmt.
			Sprintf("sid=%d",
				__jspt_spawn_41,
			))
	go func(__jspt_ctx_62 context.Context,) {
		trace.WithRegion(__jspt_ctx_62, "goroutine: anon", func() {
			{
				trace.Log(ctx,

					"spawn_child",
					fmt.
						Sprintf("sid=%d",
							__jspt_spawn_41,
						))

				defer wg.Done()
				for i := 0; i < 3; i++ {
					select {
					case <-__jspt_recv_in_region_4(ctx, "worker: receive from "+"chan", ctx.Done()):
						if trace.IsEnabled() {
							trace.Log(ctx, "select_recv",

								fmt.
									Sprintf("ptr=%p name=%q",
										ctx.Done(), "worker: receive from "+
											"chan"))
						}

						return
					default:
						time.
							Sleep(1 *
								time.Millisecond,
							)

					}
					trace.WithRegion(ctx, "rwmutex.lock rw", func() {
						{
							rw.Lock()
						}
					})
					shared += 10
					time.Sleep(35 * time.Millisecond)
					trace.WithRegion(ctx, "rwmutex.unlock rw", func() {
						{
							rw.Unlock()
						}
					})
					time.Sleep(20 * time.Millisecond)
				}
			}
		})
	}(ctx)
	trace.WithRegion(ctx, "wg.wait wg", func() {
		{

			wg.Wait()
		}
	})
}

func condDemo(ctx context.Context) {
	var mu sync.Mutex
	cond := sync.NewCond(&mu)
	ready := false

	var wg sync.WaitGroup
	trace.WithRegion(

	// Two waiters: block on cond.Wait
	ctx, "wg.add wg", func() {
		{
			wg.Add(3)
		}
	})

	for i := 0; i < 2; i++ {
		__jspt_spawn_42 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
		trace.Log(ctx,

			"spawn_parent",
			fmt.
				Sprintf("sid=%d",
					__jspt_spawn_42,
				))
		go func(__jspt_ctx_63 context.Context, id int) {
			trace.WithRegion(__jspt_ctx_63, "goroutine: anon", func() {
				{
					trace.Log(ctx,

						"spawn_child",
						fmt.
							Sprintf("sid=%d",
								__jspt_spawn_42,
							))

					defer wg.Done()
					trace.WithRegion(ctx, "mutex.lock mu", func() {
						{
							mu.Lock()
						}
					})
					for !ready {
						trace.WithRegion(ctx,
						// blocks; unblocks on Signal/Broadcast
						"cond.wait cond", func() {
							{
								cond.Wait()
							}
						})
					}
					trace.WithRegion(ctx, "mutex.unlock mu",

					// Signaler: wakes waiters
					func() {
						{
							mu.Unlock()
						}
					})
				}
			})
		}(ctx, i)
	}
	__jspt_spawn_43 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
	trace.Log(ctx,

		"spawn_parent",
		fmt.
			Sprintf("sid=%d",
				__jspt_spawn_43,
			))
	go func(__jspt_ctx_64 context.Context,) {
		trace.WithRegion(__jspt_ctx_64, "goroutine: anon", func() {
			{
				trace.Log(ctx,

					"spawn_child",
					fmt.
						Sprintf("sid=%d",
							__jspt_spawn_43,
						))

				defer wg.Done()
				{
					__jspt_sel_44 := fmt.
						Sprintf(
							"s%v", time.Now().UnixNano())
					if trace.IsEnabled() {
						trace.Log(ctx, "select",
							"select_begin id="+
								__jspt_sel_44,
						)
					}

					select {
					case <-__jspt_recv_in_region_4(ctx, "worker: receive from "+"chan", ctx.Done()):
						if trace.IsEnabled() {
							trace.Log(ctx, "select",
								fmt.Sprintf("select_chosen id=%s idx=0",
									__jspt_sel_44))
						}
						if trace.IsEnabled() {
							trace.Log(ctx, "select",
								fmt.Sprintf("select_case id=%s idx=0 kind=recv ptr=%p name=%q",

									__jspt_sel_44, ctx.Done(), "worker: receive from "+"chan"))
						}
						if trace.IsEnabled() {
							trace.Log(ctx, "select_recv",

								fmt.
									Sprintf("ptr=%p name=%q",
										ctx.Done(), "worker: receive from "+
											"chan"))
						}

						return
					case <-time.After(180 * time.Millisecond):
					}
					if trace.IsEnabled() {
						trace.Log(ctx, "select",
							"select_end id="+
								__jspt_sel_44,
						)
					}
				}
				trace.WithRegion(ctx, "mutex.lock mu", func() {
					{

						mu.Lock()
					}
				})
				ready = true
				trace.WithRegion(ctx, "mutex.unlock mu",

				// unblock multiple waiters
				func() {
					{
						mu.Unlock()
					}
				})
				trace.WithRegion(ctx, "cond.broadcast cond", func() {
					{

						cond.Broadcast()
					}
				})
			}
		})
	}(ctx)
	trace.WithRegion(ctx, "wg.wait wg", func() {
		{

			wg.Wait()
		}
	})
}
func __jspt_select_recv_3[T any](__jspt_ctx_65 context.
	Context, label string, ch <-chan T) <-chan T {
	if trace.IsEnabled() {
		trace.
			Log(__jspt_ctx_65,

				"ch_ptr", fmt.Sprintf("ptr=%p", ch))
		trace.Log(__jspt_ctx_65, "ch_name", fmt.Sprintf("ptr=%p name=%s",

			ch, label))
	}
	return ch
}
func __jspt_recv_in_region_4[T any](__jspt_ctx_65 context.Context, label string, ch <-chan T) <-chan T {
	if trace.
		IsEnabled() {
		trace.Log(__jspt_ctx_65,
			"ch_ptr", fmt.Sprintf(
				"ptr=%p", ch))
		trace.Log(__jspt_ctx_65, "ch_name", fmt.Sprintf("ptr=%p name=%s",
			ch,
			label))
	}
	return ch
}
func __jspt_select_send_5[T any](__jspt_ctx_65 context.Context, label string, ch chan<- T) chan<- T {
	if trace.IsEnabled() {
		trace.
			Log(__jspt_ctx_65,
				"ch_ptr", fmt.
					Sprintf("ptr=%p",
						ch),
			)
		trace.Log(__jspt_ctx_65, "ch_name", fmt.Sprintf("ptr=%p name=%s",

			ch, label,
		))
	}
	return ch
}
func init() {
	RegisterWorkload("richsample",
		RunrichsampleProgram,
	)
}
