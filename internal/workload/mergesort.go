package workload

import (
	"context"
	"fmt"
	"runtime/trace"
	"time"
)

// RunMergeSortProgram runs a merge-sort demo workload using the provided context.
// The caller is responsible for managing trace start/stop and task scope.
func RunMergeSortProgram(ctx context.Context) {
	// Create a task so the trace groups everything under "merge-sort-program".
	ctx, task := trace.NewTask(ctx, "merge-sort-program")
	defer task.End()

	trace.Log(ctx, "main", "creating input slice")

	// Demo data – feel free to change it.
	data := []int{9, 1, 5, 7, 3, 8, 6, 2, 4}
	trace.Log(ctx, "main", fmt.Sprintf("input: %v", data))

	// Region for the overall sort.
	region := trace.StartRegion(ctx, "merge-sort")
	sorted := MergeSort(ctx, 0, data)
	region.End()

	trace.Log(ctx, "main", fmt.Sprintf("sorted: %v", sorted))

	// Short sleep so the trace has time to capture the last events
	// before the program exits.
	time.Sleep(50 * time.Millisecond)
}

// MergeSort performs a concurrent merge sort on the given slice.
// It uses trace regions and logs so that the visualizer can show
// goroutine activity, blocking on channels, and merge steps.
func MergeSort(ctx context.Context, depth int, data []int) []int {
	if len(data) <= 1 {
		// Copy so we don't mutate the caller's slice.
		out := make([]int, len(data))
		copy(out, data)
		return out
	}

	region := trace.StartRegion(ctx, fmt.Sprintf("merge-sort len=%d depth=%d", len(data), depth))
	defer region.End()

	mid := len(data) / 2

	left := make([]int, mid)
	right := make([]int, len(data)-mid)
	copy(left, data[:mid])
	copy(right, data[mid:])

	trace.Log(ctx, "split", fmt.Sprintf("left=%v right=%v", left, right))

	// Channels to collect results from the two recursive goroutines.
	leftCh := make(chan []int, 1)
	rightCh := make(chan []int, 1)

	// Sort left half in its own goroutine.
	go func() {
		r := trace.StartRegion(ctx, fmt.Sprintf("sort-left len=%d", len(left)))
		defer r.End()
		res := MergeSort(ctx, depth+1, left)
		trace.Log(ctx, "worker", fmt.Sprintf("sorted left=%v", res))
		TraceSend(ctx, fmt.Sprintf("worker: send to left[%d]", depth), leftCh, res)
	}()

	// Sort right half in its own goroutine.
	go func() {
		r := trace.StartRegion(ctx, fmt.Sprintf("sort-right len=%d", len(right)))
		defer r.End()
		res := MergeSort(ctx, depth+1, right)
		trace.Log(ctx, "worker", fmt.Sprintf("sorted right=%v", res))
		TraceSend(ctx, fmt.Sprintf("worker: send to right[%d]", depth), rightCh, res)
	}()

	// These receives will show up as blocking operations in the trace.
	leftSorted := TraceRecv[[]int](ctx, fmt.Sprintf("worker: receive from left[%d]", depth), leftCh)

	rightSorted := TraceRecv[[]int](ctx, fmt.Sprintf("worker: receive from right[%d]", depth), rightCh)

	mergeRegion := trace.StartRegion(ctx, "merge")
	merged := merge(leftSorted, rightSorted)
	mergeRegion.End()

	trace.Log(ctx, "merge", fmt.Sprintf("merged=%v", merged))

	return merged
}

// merge merges two sorted slices into a new sorted slice.
func merge(a, b []int) []int {
	out := make([]int, 0, len(a)+len(b))
	i, j := 0, 0

	for i < len(a) && j < len(b) {
		if a[i] <= b[j] {
			out = append(out, a[i])
			i++
		} else {
			out = append(out, b[j])
			j++
		}
	}

	out = append(out, a[i:]...)
	out = append(out, b[j:]...)
	return out
}
