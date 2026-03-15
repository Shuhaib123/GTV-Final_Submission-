package instrumenter

import (
	"strings"
	"testing"
)

func resetInstrumenterEnv(t *testing.T) {
	t.Helper()
	t.Setenv("GTV_INSTR_LEVEL", "")
	t.Setenv("GTV_INSTR_BLOCK_REGIONS", "")
	t.Setenv("GTV_INSTR_CONFIG", "")
	t.Setenv("GTV_MVP", "")
}

func TestAddBlockRegionsGatesSyncWrapping(t *testing.T) {
	resetInstrumenterEnv(t)
	prev := instrOpts
	defer SetOptions(prev)

	src := []byte(`package main
import "sync"
func main() {
	var mu sync.Mutex
	var wg sync.WaitGroup
	cond := sync.NewCond(&mu)
	mu.Lock()
	mu.Unlock()
	wg.Add(1)
	wg.Done()
	wg.Wait()
	cond.Signal()
	cond.Broadcast()
	_ = cond
}`)

	SetOptions(Options{
		Level:               "regions_logs",
		GuardDynamicLabels:  true,
		AddGoroutineRegions: true,
		AddBlockRegions:     true,
	})
	withBlocks, err := InstrumentProgram(src, "synccase")
	if err != nil {
		t.Fatalf("instrument with blocks: %v", err)
	}
	gotWith := string(withBlocks)
	wantLabels := []string{
		"mutex.lock mu",
		"mutex.unlock mu",
		"wg.add wg",
		"wg.done wg",
		"wg.wait wg",
		"cond.signal cond",
		"cond.broadcast cond",
	}
	for _, label := range wantLabels {
		if !strings.Contains(gotWith, label) {
			t.Fatalf("expected wrapped sync label %q in output", label)
		}
	}

	SetOptions(Options{
		Level:               "regions_logs",
		GuardDynamicLabels:  true,
		AddGoroutineRegions: true,
		AddBlockRegions:     false,
	})
	withoutBlocks, err := InstrumentProgram(src, "synccase")
	if err != nil {
		t.Fatalf("instrument without blocks: %v", err)
	}
	gotWithout := string(withoutBlocks)
	for _, label := range wantLabels {
		if strings.Contains(gotWithout, label) {
			t.Fatalf("did not expect sync block-region label %q when AddBlockRegions=false", label)
		}
	}
}
