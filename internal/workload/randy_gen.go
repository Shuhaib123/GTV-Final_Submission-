//go:build workload_randy
// +build workload_randy

/*
filename:  doubly-linked-list.go
author:    Lex Sheehan
copyright: Lex Sheehan LLC
license:   GPL
status:    published
comments:  http://l3x.github.io/golang-code-examples/2014/07/23/doubly-linked-list.html
*/
package workload

import (
	"fmt"
	"errors"
	"strings"
	"context"
	"runtime/trace"
	"jspt/internal/gtvtrace"
)

type Value struct {
	Name		string
	MilesAway	int
}
type Node struct {
	Value		// Embedded struct
	next, prev	*Node
}
type List struct {
	head, tail *Node
}

func (l *List) First(__jspt_ctx_10 context.Context,) *Node {
	return l.head
}
func (n *Node) Next(__jspt_ctx_11 context.Context,) *Node {
	return n.next
}
func (n *Node) Prev(__jspt_ctx_12 context.Context,) *Node {
	return n.prev
}

// Create new node with value
func (l *List) Push(__jspt_ctx_13 context.Context, v Value) *List {
	n := &Node{Value: v}
	if l.head == nil {
		l.head = n	// First node
	} else {
		l.tail.next = n	// Add after prev last node
		n.prev = l.tail	// Link back to prev last node
	}
	l.tail = n	// reset tail to newly added node
	return l
}
func (l *List) Find(__jspt_ctx_14 context.Context, name string) *Node {
	found := false
	var ret *Node = nil
	for n := l.First(__jspt_ctx_14); n != nil && !found; n = n.Next(__jspt_ctx_14) {
		if n.Value.Name == name {
			found = true
			ret = n
		}
	}
	return ret
}
func (l *List) Delete(__jspt_ctx_15 context.Context, name string) bool {
	success := false
	node2del := l.Find(__jspt_ctx_15, name)
	if node2del != nil {
		fmt.Println("Delete - FOUND: ", name)
		prev_node := node2del.prev
		next_node := node2del.next
		// Remove this node
		prev_node.next = node2del.next
		next_node.prev = node2del.prev
		success = true
	}
	return success
}

var errEmpty = errors.New("ERROR - List is empty")

// Pop last item from list
func (l *List) Pop(__jspt_ctx_16 context.Context,) (v Value, err error) {
	if l.tail == nil {
		err = errEmpty
	} else {
		v = l.tail.Value
		l.tail = l.tail.prev
		if l.tail == nil {
			l.head = nil
		}
	}
	return v, err
}

func RunrandyProgram(__jspt_ctx_8 context.Context) {
	gtvtrace.
		InstallStopOnSignal()
	gtvtrace.
		InstallStopAfterFromEnv("GTV_TIMEOUT_MS",
		)
	__jspt_ctx_8, __jspt_task_9 := trace.NewTask(__jspt_ctx_8, "randy")
	defer __jspt_task_9.End()
	trace.Log(__jspt_ctx_8, "main", "randy starting")

	dashes := strings.Repeat("-", 50)
	l := new(List)	// Create Doubly Linked List

	l.Push(__jspt_ctx_8, Value{Name: "Atlanta", MilesAway: 0})
	l.Push(__jspt_ctx_8, Value{Name: "Las Vegas", MilesAway: 1961})
	l.Push(__jspt_ctx_8, Value{Name: "New York", MilesAway: 881})

	processed := make(map[*Node]bool)

	fmt.Println("First time through list...")
	for n := l.First(__jspt_ctx_8); n != nil; n = n.Next(__jspt_ctx_8) {
		fmt.Printf("%v\n", n.Value)
		if processed[n] {
			fmt.Printf("%s as been processed\n", n.Value)
		}
		processed[n] = true
	}
	fmt.Println(dashes)
	fmt.Println("Second time through list...")
	for n := l.First(__jspt_ctx_8); n != nil; n = n.Next(__jspt_ctx_8) {
		fmt.Printf("%v", n.Value)
		if processed[n] {
			fmt.Println(" has been processed")
		} else {
			fmt.Println()
		}
		processed[n] = true
	}

	fmt.Println(dashes)
	var found_node *Node
	city_to_find := "New York"
	found_node = l.Find(__jspt_ctx_8, city_to_find)
	if found_node == nil {
		fmt.Printf("NOT FOUND: %v\n", city_to_find)
	} else {
		fmt.Printf("FOUND: %v\n", city_to_find)
	}

	city_to_find = "Chicago"
	found_node = l.Find(__jspt_ctx_8, city_to_find)
	if found_node == nil {
		fmt.Printf("NOT FOUND: %v\n", city_to_find)
	} else {
		fmt.Printf("FOUND: %v\n", city_to_find)
	}

	fmt.Println(dashes)
	city_to_remove := "Las Vegas"
	successfully_removed_city := l.Delete(__jspt_ctx_8, city_to_remove)
	if successfully_removed_city {
		fmt.Printf("REMOVED: %v\n", city_to_remove)
	} else {
		fmt.Printf("DID NOT REMOVE: %v\n", city_to_remove)
	}

	city_to_remove = "Chicago"
	successfully_removed_city = l.Delete(__jspt_ctx_8, city_to_remove)
	if successfully_removed_city {
		fmt.Printf("REMOVED: %v\n", city_to_remove)
	} else {
		fmt.Printf("DID NOT REMOVE: %v\n", city_to_remove)
	}

	fmt.Println(dashes)
	fmt.Println("* Pop each value off list...")
	for v, err := l.Pop(__jspt_ctx_8); err == nil; v, err = l.Pop(__jspt_ctx_8) {
		fmt.Printf("%v\n", v)
	}
	fmt.Println(l.Pop(__jspt_ctx_8))	// Generate error - attempt to pop from empty list
}
func init() {
	RegisterWorkload("randy", RunrandyProgram,
	)
}
