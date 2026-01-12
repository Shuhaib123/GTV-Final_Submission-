package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("go.mod not found from %s", dir)
		}
		dir = parent
	}
}

func runCmd(t *testing.T, root string, env []string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = root
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("cmd failed: %v\n%s", err, string(out))
	}
}

func TestTimeoutProducesJSON(t *testing.T) {
	root := findRepoRoot(t)
	tmp := t.TempDir()
	gocache := filepath.Join(tmp, "gocache")
	env := append(os.Environ(), "GOCACHE="+gocache)

	fixture := filepath.Join(root, "internal", "testdata", "infinite", "main.go")
	outPath := filepath.Join(root, "internal", "workload", "infinite_gen.go")
	_ = os.Remove(outPath)

	runCmd(t, root, env, "go", "run", "./cmd/gtv-instrument", "-in", fixture, "-name", "Infinite")

	traceOut := filepath.Join(tmp, "trace.out")
	{
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "go", "run", "-tags", "workload_infinite", "./cmd/gtv-runner", "-workload", "infinite", "-timeout", "200ms")
		cmd.Dir = root
		cmd.Env = env
		outFile, err := os.Create(traceOut)
		if err != nil {
			t.Fatalf("create trace.out: %v", err)
		}
		defer outFile.Close()
		cmd.Stdout = outFile
		var errBuf bytes.Buffer
		cmd.Stderr = &errBuf
		err = cmd.Run()
		if err != nil {
			t.Fatalf("runner failed: %v\n%s", err, errBuf.String())
		}
	}

	info, err := os.Stat(traceOut)
	if err != nil {
		t.Fatalf("stat trace.out: %v", err)
	}
	if info.Size() == 0 {
		t.Fatalf("trace.out is empty")
	}

	traceJSON := filepath.Join(tmp, "trace.json")
	runCmd(t, root, env, "go", "run", ".", "-parse", traceOut, "-json", traceJSON)

	raw, err := os.ReadFile(traceJSON)
	if err != nil {
		t.Fatalf("read trace.json: %v", err)
	}
	var events []map[string]any
	if err := json.Unmarshal(raw, &events); err != nil {
		t.Fatalf("json parse failed: %v", err)
	}
	if len(events) == 0 {
		t.Fatalf("trace.json has no events")
	}

	_ = os.Remove(outPath)
}
