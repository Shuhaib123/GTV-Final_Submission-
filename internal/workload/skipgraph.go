package workload

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"os"
	"runtime/trace"
	"sort"
	"strconv"
	"time"
)

// Faithful SkipGraph-style workload (bounded) with full instrumentation.
// Channel nodes are named in[<id>] so the parser can pair send/recv.

// ==== Env helpers ====
func sgEnvInt(name string, def int) int {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

// ==== Instrumentation helpers ====
func inboxLabel(id uint32) string { return fmt.Sprintf("in[%d]", id) }

func sendTo(ctx context.Context, dst *linearize, payload *linearize) {
	TraceSend(ctx, "worker: send to "+inboxLabel(dst.value), *dst.inputChannel, payload)
}

func recvFromSelf(ctx context.Context, self *node) {
	trace.WithRegion(ctx, "worker: receive from "+inboxLabel(self.value), func() {})
}

// ==== Data model (matching the original) ====

type levelRange struct {
	level        uint8
	bitstring    *string
	value        uint32
	borderNodes  [4]*linearize
	nodesInRange []*linearize
}

func initLevelRange(v uint32, bs *string, l uint8) *levelRange {
	lr := &levelRange{level: l, bitstring: bs, value: v}
	lr.borderNodes = [4]*linearize{nil, nil, nil, nil}
	lr.nodesInRange = make([]*linearize, 0)
	return lr
}

type linearizeCount struct {
	linearizePackage *linearize
	count            uint8
}

type node struct {
	value                    uint32
	bitstring                *string
	ranges                   []*levelRange
	communicationChannel     chan *linearize
	nodesConnected           []*linearizeCount
	listOfDeletedNodes       []*linearizeCount
	handledLinearizePackages uint32
	changedSinceTimeout      bool
}

func generateBitstring(length uint8) string {
	b := make([]byte, length)
	for i := uint8(0); i < length; i++ {
		if rand.Intn(2) == 1 {
			b[i] = '1'
		} else {
			b[i] = '0'
		}
	}
	return string(b)
}

func initNode(v uint32, depth uint8) *node {
	n := new(node)
	n.value = v
	bs := generateBitstring(depth)
	n.bitstring = &bs
	n.ranges = make([]*levelRange, 0)
	for d := uint8(0); d < depth; d++ {
		n.ranges = append(n.ranges, initLevelRange(n.value, n.bitstring, d))
	}
	n.communicationChannel = make(chan *linearize, 1000)
	n.nodesConnected = make([]*linearizeCount, 0)
	n.listOfDeletedNodes = make([]*linearizeCount, 0)
	n.handledLinearizePackages = 0
	n.changedSinceTimeout = false
	return n
}

func removeNodeFromRanges(n *node, lP *linearize) {
	for _, r := range n.ranges {
		newRangeBorders := [4]*linearize{nil, nil, nil, nil}
		for i, nR := range r.borderNodes {
			if nR != nil {
				if nR.value != lP.value && *nR.bitstring != *lP.bitstring {
					newRangeBorders[i] = nR
				}
			}
		}
		r.borderNodes = newRangeBorders
		newNodesInRange := make([]*linearize, 0)
		for _, nR := range r.nodesInRange {
			if nR.value != lP.value || *nR.bitstring != *lP.bitstring {
				newNodesInRange = append(newNodesInRange, nR)
			}
		}
		r.nodesInRange = newNodesInRange
		n.listOfDeletedNodes = append(n.listOfDeletedNodes, &linearizeCount{lP, 255})
	}
	newNodesConnected := make([]*linearizeCount, 0)
	for _, nC := range n.nodesConnected {
		if nC.linearizePackage.value != lP.value || *nC.linearizePackage.bitstring != *lP.bitstring {
			newNodesConnected = append(newNodesConnected, nC)
		}
	}
	n.nodesConnected = newNodesConnected
}

type linearize struct {
	inputChannel    *chan *linearize
	value           uint32
	bitstring       *string
	search          bool
	searchBitstring bool
	leaveGraph      bool
}

func initLinearize(node *node) *linearize {
	return &linearize{&node.communicationChannel, node.value, node.bitstring, false, false, false}
}
func initLeaveLinearize(n *node) *linearize {
	return &linearize{&n.communicationChannel, n.value, n.bitstring, false, false, true}
}
func initSearchLinearizePackage(node *node, bitstring bool) *linearize {
	return &linearize{nil, node.value, node.bitstring, true, bitstring, false}
}

// ==== Node actor + maintenance ====

func manageNode(ctx context.Context, n *node, timeoutLength time.Duration) {
	trace.Log(ctx, "worker", fmt.Sprintf("node %d starting", n.value))
	timeoutTicker := time.NewTicker(timeoutLength)
	for {
		select {
		case <-ctx.Done():
			return
		case lP := <-n.communicationChannel:
			recvFromSelf(ctx, n)
			n.handledLinearizePackages++
			if lP == nil {
				// Leave event: broadcast to neighbors and echo to stragglers
				for _, nR := range n.nodesConnected {
					sendTo(ctx, nR.linearizePackage, initLeaveLinearize(n))
				}
				for {
					select {
					case lPN := <-n.communicationChannel:
						recvFromSelf(ctx, n)
						if lPN != nil && lPN.inputChannel != nil {
							dst := &linearize{inputChannel: lPN.inputChannel, value: lPN.value, bitstring: lPN.bitstring}
							sendTo(ctx, dst, initLeaveLinearize(n))
						}
					default:
						// stop echoing when inbox is drained
						goto postLeave
					}
				}
			postLeave:
			} else if lP.leaveGraph {
				removeNodeFromRanges(n, lP)
			} else if lP.search {
				if lP.searchBitstring {
					searchOnNodeBitstring(ctx, n, lP)
				} else {
					searchOnNodeID(ctx, n, lP)
				}
			} else {
				deleted := false
				for _, dN := range n.listOfDeletedNodes {
					if dN.linearizePackage.value == lP.value && *dN.linearizePackage.bitstring == *lP.bitstring {
						deleted = true
						if dN.count > 0 {
							dN.count--
						}
					}
				}
				if !deleted {
					droppedList := updateRangesOfNode(n, lP)
					for _, d := range droppedList {
						next := getBestFitNext(n, d)
						sendTo(ctx, next, d)
					}
				}
			}
		case <-timeoutTicker.C:
			n.changedSinceTimeout = false
			timeout(ctx, n)
		}
	}
}

func timeout(ctx context.Context, n *node) {
	n.handledLinearizePackages = 0
	for _, r := range n.ranges {
		linearizeRange(ctx, r, n)
	}
}

func preventNilBorder(lP *linearize, floor bool) uint32 {
	if lP == nil {
		if floor {
			return 0
		}
		return math.MaxUint32
	}
	return lP.value
}
func specialMin(a, b uint32) uint32 {
	if a == 0 {
		return b
	}
	if b == 0 {
		return a
	}
	if a < b {
		return a
	}
	return b
}
func specialMax(a, b uint32) uint32 {
	if a == math.MaxUint32 {
		return b
	}
	if b == math.MaxUint32 {
		return a
	}
	if a < b {
		return b
	}
	return a
}

func getRangeBorders(lR *levelRange) [2]uint32 {
	return [2]uint32{specialMin(preventNilBorder(lR.borderNodes[0], true), preventNilBorder(lR.borderNodes[2], true)), specialMax(preventNilBorder(lR.borderNodes[1], false), preventNilBorder(lR.borderNodes[3], false))}
}

func updateRange(currentRange *levelRange, lP *linearize) []*linearize {
	droppedList := make([]*linearize, 0)
	// part of the range?
	lvl := int(currentRange.level)
	if lP.value < preventNilBorder(currentRange.borderNodes[0], true) && lP.value < preventNilBorder(currentRange.borderNodes[2], true) ||
		lP.value > preventNilBorder(currentRange.borderNodes[1], false) && lP.value > preventNilBorder(currentRange.borderNodes[3], false) ||
		(*currentRange.bitstring)[:lvl] != (*lP.bitstring)[:lvl] {
		droppedList = append(droppedList, lP)
	} else if lP.value != currentRange.value {
		// ensure unique in nodesInRange
		found := false
		for _, node := range currentRange.nodesInRange {
			if node.value == lP.value {
				found = true
				break
			}
		}
		if !found {
			currentRange.nodesInRange = append(currentRange.nodesInRange, lP)
		}
		// borders
		if (*lP.bitstring)[lvl] == '0' {
			if lP.value > preventNilBorder(currentRange.borderNodes[0], true) && lP.value < currentRange.value {
				currentRange.borderNodes[0] = lP
			} else if lP.value < preventNilBorder(currentRange.borderNodes[1], false) && lP.value > currentRange.value {
				currentRange.borderNodes[1] = lP
			}
		} else {
			if lP.value > preventNilBorder(currentRange.borderNodes[2], true) && lP.value < currentRange.value {
				currentRange.borderNodes[2] = lP
			} else if lP.value < preventNilBorder(currentRange.borderNodes[3], false) && lP.value > currentRange.value {
				currentRange.borderNodes[3] = lP
			}
		}
	}
	rangeBorders := getRangeBorders(currentRange)
	for _, node := range currentRange.nodesInRange {
		if node.value < rangeBorders[0] || node.value > rangeBorders[1] {
			droppedList = append(droppedList, node)
		}
	}
	// cleanup
	kept := make([]*linearize, 0, len(currentRange.nodesInRange))
	for _, node := range currentRange.nodesInRange {
		if node != nil {
			if !(node.value < rangeBorders[0] || node.value > rangeBorders[1]) {
				kept = append(kept, node)
			}
		}
	}
	currentRange.nodesInRange = kept
	return droppedList
}

func sortRange(currentRange *levelRange) {
	sort.Slice(currentRange.nodesInRange, func(i, j int) bool { return currentRange.nodesInRange[i].value < currentRange.nodesInRange[j].value })
}

func updateRangesOfNode(n *node, lP *linearize) []*linearize {
	droppedList := make([]*linearize, 0)
	inList := false
	for _, connectedNode := range n.nodesConnected {
		if connectedNode.linearizePackage.value == lP.value {
			inList = true
			connectedNode.count = uint8(len(n.ranges))
		}
	}
	if !inList {
		n.nodesConnected = append(n.nodesConnected, &linearizeCount{lP, uint8(len(n.ranges))})
	}
	for _, r := range n.ranges {
		droppedListRange := updateRange(r, lP)
		for _, dP := range droppedListRange {
			for _, cP := range n.nodesConnected {
				if cP.linearizePackage.value == dP.value {
					if cP.count > 0 {
						cP.count--
					}
				}
			}
		}
	}
	newNodesConnected := make([]*linearizeCount, 0)
	for _, cP := range n.nodesConnected {
		if cP.count == 0 {
			droppedList = append(droppedList, cP.linearizePackage)
		} else {
			newNodesConnected = append(newNodesConnected, cP)
		}
	}
	n.nodesConnected = newNodesConnected
	if len(droppedList) > 0 || !inList {
		n.changedSinceTimeout = true
	}
	return droppedList
}

func distance(num1 uint32, num2 uint32) uint32 {
	if num1 > num2 {
		return num1 - num2
	}
	return num2 - num1
}

func getFitLevel(string1 *string, string2 *string) int {
	level := -1
	max := len(*string1)
	if len(*string2) < max {
		max = len(*string2)
	}
	for i := 0; i < max; i++ {
		if (*string1)[i] == (*string2)[i] {
			level++
		} else {
			break
		}
	}
	return level
}

func getBestFitNext(node *node, lP *linearize) *linearize {
	result := initLinearize(node)
	var currentDistance uint32 = math.MaxUint32
	bestFitBitstring := -1
	for _, n := range node.nodesConnected {
		fl := getFitLevel(n.linearizePackage.bitstring, lP.bitstring)
		if fl > bestFitBitstring || (fl == bestFitBitstring && distance(n.linearizePackage.value, lP.value) < currentDistance) {
			result = n.linearizePackage
			currentDistance = distance(n.linearizePackage.value, lP.value)
			bestFitBitstring = fl
		}
	}
	return result
}

func linearizeRange(ctx context.Context, currentRange *levelRange, currentNode *node) {
	sortRange(currentRange)
	passedMiddle := false
	middleUpper := -1
	for i, nd := range currentRange.nodesInRange {
		if nd.value > currentNode.value && !passedMiddle {
			middleUpper = i
			passedMiddle = true
			sendTo(ctx, nd, initLinearize(currentNode))
			if i > 0 {
				sendTo(ctx, currentRange.nodesInRange[i-1], initLinearize(currentNode))
			}
		}
		if i > 0 {
			if !passedMiddle {
				sendTo(ctx, currentRange.nodesInRange[i-1], nd)
			} else {
				sendTo(ctx, nd, currentRange.nodesInRange[i-1])
			}
		}
	}
	if len(currentRange.nodesInRange) > 0 && !passedMiddle {
		sendTo(ctx, currentRange.nodesInRange[len(currentRange.nodesInRange)-1], initLinearize(currentNode))
	}
	if middleUpper != -1 {
		for _, nd := range currentRange.nodesInRange {
			if nd.value < currentNode.value {
				sendTo(ctx, nd, currentRange.nodesInRange[middleUpper])
			} else if middleUpper > 0 {
				sendTo(ctx, nd, currentRange.nodesInRange[middleUpper-1])
			}
		}
	}
	if currentRange.borderNodes[0] != nil && currentRange.borderNodes[3] != nil {
		sendTo(ctx, currentRange.borderNodes[0], currentRange.borderNodes[3])
		sendTo(ctx, currentRange.borderNodes[3], currentRange.borderNodes[0])
	}
	if currentRange.borderNodes[1] != nil && currentRange.borderNodes[2] != nil {
		sendTo(ctx, currentRange.borderNodes[1], currentRange.borderNodes[2])
		sendTo(ctx, currentRange.borderNodes[2], currentRange.borderNodes[1])
	}
}

// Searches (forwarding instrumented)
func searchOnNodeBitstring(ctx context.Context, n *node, lP *linearize) {
	if *lP.bitstring == *n.bitstring {
		return
	}
	currentBest := initLinearize(n)
	for _, nd := range n.nodesConnected {
		if getFitLevel(nd.linearizePackage.bitstring, lP.bitstring) > getFitLevel(currentBest.bitstring, lP.bitstring) {
			currentBest = nd.linearizePackage
		}
	}
	if *currentBest.bitstring != *n.bitstring {
		sendTo(ctx, currentBest, lP)
	}
}

func searchOnNodeID(ctx context.Context, n *node, lP *linearize) {
	if n.value == lP.value {
		return
	}
	currentIndex := -1
	currentValue := n.value
	if lP.value > n.value {
		sort.Slice(n.nodesConnected, func(i, j int) bool {
			return n.nodesConnected[i].linearizePackage.value < n.nodesConnected[j].linearizePackage.value
		})
		for i, nd := range n.nodesConnected {
			if nd.linearizePackage.value > currentValue && nd.linearizePackage.value <= lP.value {
				currentIndex = i
			}
		}
	} else {
		sort.Slice(n.nodesConnected, func(i, j int) bool {
			return n.nodesConnected[i].linearizePackage.value > n.nodesConnected[j].linearizePackage.value
		})
		for i, nd := range n.nodesConnected {
			if nd.linearizePackage.value < currentValue && nd.linearizePackage.value >= lP.value {
				currentIndex = i
			}
		}
	}
	if currentIndex != -1 {
		sendTo(ctx, n.nodesConnected[currentIndex].linearizePackage, lP)
	}
}

// ==== Bounded run for workload entry ====
func checkStable(nodes []*node, timeoutLength time.Duration) time.Duration {
	j := 0
	ok := false
	for rep := 0; rep < 3; rep++ {
		for {
			j++
			time.Sleep(timeoutLength)
			for i, node := range nodes {
				if node.changedSinceTimeout {
					break
				} else if i == len(nodes)-1 {
					ok = true
				}
			}
			if ok {
				break
			}
		}
	}
	return time.Duration(j * int(timeoutLength))
}

func testRun(ctx context.Context, size uint, bitstringLength uint8, timeout time.Duration) {
	listOfNodes := make([]*node, 0, size)
	anchorNode := initNode(rand.Uint32(), bitstringLength)
	go manageNode(ctx, anchorNode, timeout)
	listOfNodes = append(listOfNodes, anchorNode)
	for i := 1; i < int(size); i++ {
		tmp := initNode(rand.Uint32(), bitstringLength)
		TraceSend(ctx, "main: send to "+inboxLabel(anchorNode.value), anchorNode.communicationChannel, initLinearize(tmp))
		anchorNode = tmp
		go manageNode(ctx, anchorNode, timeout)
		listOfNodes = append(listOfNodes, anchorNode)
	}
	_ = checkStable(listOfNodes, timeout)

	// Trigger a couple of leaves (bounded)
	if len(listOfNodes) >= 4 {
		TraceSend[*linearize](ctx, "main: send to "+inboxLabel(listOfNodes[0].value), listOfNodes[0].communicationChannel, nil)
		TraceSend[*linearize](ctx, "main: send to "+inboxLabel(listOfNodes[3].value), listOfNodes[3].communicationChannel, nil)
		time.Sleep(timeout)
	}

	// Trigger a handful of searches (bitstring then id)
	for _, r := range listOfNodes {
		dst := listOfNodes[rand.Intn(len(listOfNodes))]
		TraceSend(ctx, "main: send to "+inboxLabel(dst.value), dst.communicationChannel, initSearchLinearizePackage(r, true))
		time.Sleep(3 * time.Millisecond)
	}
	for _, r := range listOfNodes {
		dst := listOfNodes[rand.Intn(len(listOfNodes))]
		TraceSend(ctx, "main: send to "+inboxLabel(dst.value), dst.communicationChannel, initSearchLinearizePackage(r, false))
		time.Sleep(3 * time.Millisecond)
	}
}

// Public entry point used by the visualizer
func RunSkipGraphFullProgram(ctx context.Context) {
	size := sgEnvInt("GTV_SG_SIZE", 16)
	depth := sgEnvInt("GTV_SG_DEPTH", 6)
	toutMs := sgEnvInt("GTV_SG_TIMEOUT_MS", 4)

	trace.Log(ctx, "main", "skipgraph_full starting")
	testRun(ctx, uint(size), uint8(depth), time.Duration(toutMs)*time.Millisecond)
}
