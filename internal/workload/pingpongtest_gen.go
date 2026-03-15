//go:build workload_pingpongtest
// +build workload_pingpongtest

package workload

import (
	"context"
	"fmt"

	// send-only channel: chan<-
	// receive-only channel: <-chan
	"jspt/internal/gtvtrace"
	"runtime/trace"
	"sync"
	"sync/atomic"
)

var __jspt_spawn_id_2 uint64

func pingpong(__jspt_ctx_10 context.Context, ping <-chan int, pong chan<- int) {
	__jspt_recv_value_6(__jspt_ctx_10, "worker: receive from "+"ping", ping)

	fmt.Println("Ping received.")
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_10,
			"value", fmt.Sprint(1))
	}
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_10,
			"ch_ptr", fmt.Sprintf("ptr=%p",

				pong))
	}
	trace.WithRegion(__jspt_ctx_10, "worker: send to "+"pong", func() {
		{

			pong <- 1
		}
	})
}

func RunpingpongtestProgram( // main Goroutine emulates pong-part
	__jspt_ctx_8 context.Context) {
	var __jspt_wg_0 sync.WaitGroup
	defer __jspt_wg_0.Wait()
	gtvtrace.InstallStopOnSignal()
	gtvtrace.InstallStopAfterFromEnv(
		"GTV_TIMEOUT_MS")
	__jspt_ctx_8, __jspt_task_9 := trace.NewTask(__jspt_ctx_8, "pingpongtest")
	defer __jspt_task_9.End()
	trace.Log(__jspt_ctx_8, "main", "pingpongtest starting")

	ping := make(chan int)
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_8,
			"chan_make", fmt.Sprintf("ptr=%p cap=%d type=%s",

				ping, 0, "int"))
	}

	pong := make(chan int)
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_8,
			"chan_make", fmt.Sprintf("ptr=%p cap=%d type=%s",

				pong, 0, "int"))
	}
	__jspt_spawn_11 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
	trace.Log(__jspt_ctx_8,

		"spawn_parent",

		fmt.Sprintf("sid=%d",
			__jspt_spawn_11))
	__jspt_wg_0.Add(1)
	go func(__jspt_ctx_12 context.Context) {
		trace.WithRegion(__jspt_ctx_12, "goroutine: anon", func() {
			{
				defer __jspt_wg_0.Done()
				trace.Log(__jspt_ctx_8,

					"spawn_child",

					fmt.Sprintf("sid=%d", __jspt_spawn_11))

				pingpong(__jspt_ctx_8, ping, pong)
			}
		})
	}(__jspt_ctx_8)
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_8,
			"value", fmt.Sprint(1))
	}
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_8,
			"ch_ptr", fmt.Sprintf("ptr=%p",

				ping))
	}
	trace.WithRegion(__jspt_ctx_8, "worker: send to "+"ping", func() {
		{

			ping <- 1
		}
	})
	__jspt_recv_value_6(__jspt_ctx_8, "worker: receive from "+"pong", pong)

	fmt.Println("Pong received.")
}
func __jspt_recv_value_6[T any](__jspt_ctx_13 context.Context, label string, ch <-chan T) T {
	if trace.IsEnabled() {
		trace.
			Log(__jspt_ctx_13, "ch_ptr", fmt.Sprintf("ptr=%p", ch))
		trace.Log(__jspt_ctx_13,
			"ch_name", fmt.Sprintf("ptr=%p name=%s", ch,
				label))
	}
	var v T
	trace.WithRegion(__jspt_ctx_13, label, func() {
		v = <-ch
	})
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_13, "value", fmt.Sprint(v))
	}
	return v
}
func __jspt_recv_value_ok_7[T any](__jspt_ctx_13 context.Context, label string, ch <-chan T) (T, bool) {
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_13, "ch_ptr", fmt.Sprintf("ptr=%p", ch))
		trace.Log(__jspt_ctx_13, "ch_name", fmt.Sprintf("ptr=%p name=%s", ch, label))
	}
	var v T
	var ok bool
	trace.WithRegion(__jspt_ctx_13, label, func() {
		v, ok = <-ch
	})
	if ok && trace.IsEnabled() {
		trace.Log(__jspt_ctx_13, "value", fmt.Sprint(v))
	}
	return v, ok
}
func init() {
	RegisterWorkload("pingpongtest",

		RunpingpongtestProgram)
}
