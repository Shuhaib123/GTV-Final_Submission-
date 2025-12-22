package traceproc

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"jspt/internal/instrumenter"

	xtrace "golang.org/x/exp/trace"
)

var exampleWorkloads = []struct {
	tag  string
	path string
}{
	{"pingpong", "examples/pingpong/main.go"},
	{"pipeline", "examples/pipeline/main.go"},
	{"selectheavy", "examples/selectheavy/main.go"},
	{"workerpool", "examples/workerpool/main.go"},
}

func TestCI_ExampleTopologies(t *testing.T) {
	if testing.Short() {
		t.Skip("skip CI workloads in -short")
	}
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("find repo root: %v", err)
	}
	for _, ex := range exampleWorkloads {
		ex := ex
		t.Run(ex.tag, func(t *testing.T) {
			workloadFile := instrumentExampleWorkload(t, repoRoot, ex.tag, ex.path)
			defer os.Remove(workloadFile)
			bin := buildExampleRunner(t, repoRoot, ex.tag)
			events := runExampleBinary(t, bin, ex.tag)
			if err := ValidateTopology(events); err != nil {
				t.Fatalf("topology validation failed for %s: %v", ex.tag, err)
			}
		})
	}
}

func instrumentExampleWorkload(t *testing.T, repoRoot, tag, src string) string {
	t.Helper()
	srcPath := filepath.Join(repoRoot, filepath.Clean(src))
	data, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("read %s: %v", srcPath, err)
	}
	instrumenter.SetOptions(instrumenter.Options{
		GuardDynamicLabels: true,
		Level:              "regions_logs",
	})
	code, err := instrumenter.InstrumentProgram(data, workloadProgramName(tag))
	if err != nil {
		t.Fatalf("instrument %s: %v", srcPath, err)
	}
	header := fmt.Sprintf("//go:build workload_%s\n// +build workload_%s\n\n", tag, tag)
	path := filepath.Join(repoRoot, "internal", "workload", fmt.Sprintf("ci_%s_gen.go", tag))
	final := append([]byte(header), code...)
	if err := os.WriteFile(path, final, 0o644); err != nil {
		t.Fatalf("write workload %s: %v", path, err)
	}
	return path
}

func workloadProgramName(tag string) string {
	if tag == "" {
		return ""
	}
	return strings.ToUpper(tag[:1]) + tag[1:]
}

func buildExampleRunner(t *testing.T, repoRoot, tag string) string {
	t.Helper()
	tmpDir := filepath.Join(repoRoot, fmt.Sprintf("tmp_gtv_example_runner_%s", tag))
	_ = os.RemoveAll(tmpDir)
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		t.Fatalf("mkdir tmp runner: %v", err)
	}
	mainSrc := fmt.Sprintf(`package main
import (
	"context"
	"log"
	"os"
	"runtime/trace"
	"time"

	"jspt/internal/workload"
)
func main() {
	tf := os.Getenv("GTV_TRACE_FILE")
	var f *os.File
	var err error
	if tf != "" {
		f, err = os.Create(tf)
		if err != nil {
			log.Fatalf("create trace: %%v", err)
		}
		if err := trace.Start(f); err != nil {
			log.Fatalf("trace start: %%v", err)
		}
		defer func() { trace.Stop(); f.Close() }()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if !workload.RunByName(ctx, "%[1]s") {
		log.Printf("workload %[1]s not found", "%[1]s")
	}
}
`, tag)
	mainPath := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(mainPath, []byte(mainSrc), 0o644); err != nil {
		t.Fatalf("write runner main: %v", err)
	}
	binPath := filepath.Join(repoRoot, fmt.Sprintf("tmp_gtv_example_bin_%s", tag))
	cmd := exec.Command("go", "build", fmt.Sprintf("-tags=workload_%s", tag), "-o", binPath, ".")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build runner %s: %v; out=%s", tag, err, string(out))
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(tmpDir)
		_ = os.Remove(binPath)
	})
	return binPath
}

func runExampleBinary(t *testing.T, binPath, tag string) []TimelineEvent {
	t.Helper()
	tf, err := os.CreateTemp("", fmt.Sprintf("gtv-example-%s-*.out", tag))
	if err != nil {
		t.Fatalf("create trace file: %v", err)
	}
	traceFile := tf.Name()
	_ = tf.Close()
	t.Cleanup(func() { os.Remove(traceFile) })
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binPath)
	cmd.Env = append(os.Environ(), "GTV_TRACE_FILE="+traceFile)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run example %s: %v; out=%s", tag, err, string(out))
	}
	fr, err := os.Open(traceFile)
	if err != nil {
		t.Fatalf("open trace file: %v", err)
	}
	defer fr.Close()
	reader, err := xtrace.NewReader(fr)
	if err != nil {
		t.Fatalf("new trace reader: %v", err)
	}
	st := NewParseState(Options{EmitAtomic: true})
	var events []TimelineEvent
	emit := func(ev TimelineEvent) error {
		events = append(events, ev)
		return nil
	}
	for {
		ev, err := reader.ReadEvent()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read trace event: %v", err)
		}
		if err := ProcessEvent(&ev, st, emit); err != nil {
			t.Fatalf("process event: %v", err)
		}
	}
	return events
}
