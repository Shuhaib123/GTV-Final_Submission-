//go:build workload_broadcastsmoke
// +build workload_broadcastsmoke

package workload

import (
	"context"
	"fmt"
	"jspt/gtv"
	"jspt/internal/gtvtrace"
	"runtime/trace"
	"sync"
	"sync/atomic"
)

var __jspt_spawn_id_2 uint64

func client(ctx context.Context, inbox <-chan string, done chan<- struct{}) {
	if trace.IsEnabled() {
		trace.Log(ctx, "ch_ptr", fmt.Sprintf("ptr=%p", inbox))
	}
	trace.WithRegion(
		// gtv:recv=inbox
		ctx, "worker: receive from inbox", func() {
			{

				<-inbox
			}
		})
	if trace.IsEnabled() {
		trace.Log(ctx, "ch_ptr", fmt.Sprintf("ptr=%p", done))
	}
	trace.WithRegion(ctx, "worker: send to "+"done", func() {
		{

			done <- struct{}{}
		}
	})
	_ = ctx
}

func RunBroadcastSmokeProgram(__jspt_ctx_6 context.Context) {
	var __jspt_wg_0 sync.WaitGroup
	defer __jspt_wg_0.Wait()
	gtvtrace.InstallStopOnSignal()
	gtvtrace.InstallStopAfterFromEnv("GTV_TIMEOUT_MS")
	__jspt_ctx_6, __jspt_task_7 := trace.NewTask(__jspt_ctx_6, "BroadcastSmoke")
	defer __jspt_task_7.End()
	trace.Log(__jspt_ctx_6, "main", "BroadcastSmoke starting")
	gtv.InstallStopOnSignal()
	gtv.InstallStopAfterFromEnv("GTV_TIMEOUT_MS")

	clientA := make(chan string)
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_6, "chan_make", fmt.Sprintf("ptr=%p cap=%d type=%s",
			clientA, 0, "string",
		))
	}

	clientB := make(chan string)
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_6, "chan_make", fmt.Sprintf("ptr=%p cap=%d type=%s",
			clientB, 0, "string",
		))
	}

	done := make(chan struct{}, 2)
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_6, "chan_make", fmt.Sprintf("ptr=%p cap=%d type=%s",
			done, 2, "struct{}",
		))
	}
	__jspt_spawn_8 := atomic.AddUint64(&__jspt_spawn_id_2, 1)
	trace.Log(__jspt_ctx_6,

		"spawn_parent",

		fmt.Sprintf("sid=%d", __jspt_spawn_8))
	__jspt_wg_0.Add(1)
	go func(__jspt_ctx_10 context.Context) {
		defer __jspt_wg_0.Done()
		trace.Log(__jspt_ctx_6,

			"spawn_child",

			fmt.Sprintf("sid=%d", __jspt_spawn_8))

		client(__jspt_ctx_6, clientA, done)
	}(__jspt_ctx_6)
	__jspt_spawn_9 := atomic.AddUint64(

		// gtv:send=clientA
		&__jspt_spawn_id_2, 1)
	trace.Log(__jspt_ctx_6,

		"spawn_parent",

		fmt.Sprintf("sid=%d", __jspt_spawn_9))
	__jspt_wg_0.Add(1)
	go func(__jspt_ctx_11 context.Context) {
		defer __jspt_wg_0.Done()
		trace.Log(__jspt_ctx_6,

			"spawn_child",

			fmt.Sprintf("sid=%d", __jspt_spawn_9))

		client(__jspt_ctx_6, clientB, done)
	}(__jspt_ctx_6)

	msg := "hello"
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_6, "ch_ptr", fmt.Sprintf("ptr=%p", clientA))
	}
	trace.WithRegion(__jspt_ctx_6, "worker: send to clientA", func() {
		{

			clientA <- msg
		}
	})
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_6, "ch_ptr", fmt.Sprintf("ptr=%p", clientB))
	}
	trace.WithRegion(__jspt_ctx_6, "worker: send to clientB", func() {
		{

			// gtv:send=clientB
			clientB <- msg
		}
	})
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_6, "ch_ptr", fmt.Sprintf("ptr=%p", done))
	}
	trace.WithRegion(__jspt_ctx_6, "worker: receive from "+"done", func() {
		{

			<-done
		}
	})
	if trace.IsEnabled() {
		trace.Log(__jspt_ctx_6, "ch_ptr", fmt.Sprintf("ptr=%p", done))
	}
	trace.WithRegion(__jspt_ctx_6, "worker: receive from "+"done", func() {
		{

			<-done
		}
	})
}
func init() {
	RegisterWorkload("broadcastsmoke",

		RunBroadcastSmokeProgram)
}
