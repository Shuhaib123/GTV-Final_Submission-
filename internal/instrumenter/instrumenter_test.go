package instrumenter

import (
	"flag"
	"go/format"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstrument_IO_DB_JSON_and_Loop(t *testing.T) {
	src := `package main
import (
    "context"
    "database/sql"
    "encoding/json"
)
type S struct{ A int }
func main(){
    ctx := context.Background()
    var db *sql.DB
    q := "SELECT 1"
    if false {
        _, _ = db.QueryContext(ctx, q)
        _, _ = db.ExecContext(ctx, "UPDATE t SET v=1")
        _, _ = db.Begin()
        _, _ = db.BeginTx(ctx, nil)
    }
    var s S
    b, _ := json.Marshal(s)
    _ = json.Unmarshal(b, &s)
    // gtv:loop=hot
    for i:=0;i<3;i++{
        if i==1 { continue }
        if i==2 { break }
    }
}`
	// Enable per-library IO regions; keep defaults for others
	SetOptions(Options{AddIOJSONRegions: true, AddIODBRegions: true, Level: "regions_logs"})
	out, err := InstrumentProgram([]byte(src), "Demo")
	if err != nil {
		t.Fatalf("instrument error: %v", err)
	}
	s := string(out)
	// database/sql regions
	if !strings.Contains(s, "\"db.query\"") {
		t.Errorf("expected db.query region in output; got:\n%s", s)
	}
	if !strings.Contains(s, "\"db.exec\"") {
		t.Errorf("expected db.exec region in output; got:\n%s", s)
	}
	// encoding/json regions
	if !strings.Contains(s, "\"json.marshal\"") {
		t.Errorf("expected json.marshal region in output")
	}
	if !strings.Contains(s, "\"json.unmarshal\"") {
		t.Errorf("expected json.unmarshal region in output")
	}
	// db.begin for Begin/BeginTx
	if !strings.Contains(s, "\"db.begin\"") {
		t.Errorf("expected db.begin region in output")
	}
	// forced loop region with hint
	if !strings.Contains(s, "\"loop:hot\"") {
		t.Errorf("expected loop:hot region label in output")
	}
}

func TestInstrument_IO_HTTP_OS_AssignStartEnd(t *testing.T) {
	src := `package main
import (
  "context"
  "net/http"
  "os"
)
func main(){
  ctx := context.Background()
  url := "https://example.org"
  if false {
    resp, err := http.Get(url)
    _ = resp; _ = err
    f, err2 := os.Open("x")
    _ = f; _ = err2
  }
}`
	SetOptions(Options{AddIOHTTPRegions: true, AddIOOSRegions: true, Level: "regions_logs"})
	out, err := InstrumentProgram([]byte(src), "Demo")
	if err != nil {
		t.Fatalf("instrument error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "trace.StartRegion(ctx, \"http.call\")") {
		t.Errorf("expected StartRegion http.call for http.Get := assign; got:\n%s", s)
	}
	if !strings.Contains(s, "trace.StartRegion(ctx, \"file.open\")") {
		t.Errorf("expected StartRegion file.open for os.Open := assign; got:\n%s", s)
	}
	if !strings.Contains(s, ".End()") {
		t.Errorf("expected .End() calls for StartRegion wrappers")
	}
}

// Safe loop without hint: ensure we do not add loop regions by default (unless AddLoopRegions is on).
func TestLoop_SafeWithoutHint(t *testing.T) {
	src := `package p
import "context"
func f(ctx context.Context, xs []int) {
  for i := 0; i < len(xs); i++ {
    _ = xs[i]
  }
}`
	SetOptions(Options{Level: "regions_logs"})
	out, err := InstrumentProgram([]byte(src), "Demo")
	if err != nil {
		t.Fatalf("instrument error: %v", err)
	}
	s := string(out)
	if strings.Contains(s, "\"loop:") {
		t.Fatalf("unexpected loop region without hint: %s", s)
	}
}

// Forced loop with bare return (Design B): named results allow bare return; assert region and return flag appear.
func TestLoop_ForcedWithBareReturn(t *testing.T) {
	src := `package p
import (
  "context"
  "fmt"
)
// gtv:loop=hot_return
func f(ctx context.Context, xs []int) (n int, err error) {
  for _, x := range xs {
    if x < 0 { err = fmt.Errorf("neg"); return }
  }
  return
}`
	SetOptions(Options{Level: "regions_logs"})
	out, err := InstrumentProgram([]byte(src), "Demo")
	if err != nil {
		t.Fatalf("instrument error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "\"loop:hot_return\"") {
		t.Fatalf("expected loop:hot_return region, got: %s", s)
	}
	if !strings.Contains(s, "__gtvReturn") {
		t.Fatalf("expected return flag in forced loop transform, got: %s", s)
	}
}

// Forced loop with disallowed control flow (goto/labelled): expect no loop region emitted.
func TestLoop_ForcedWithGoto_Disallowed(t *testing.T) {
	src := `package p
import "context"
// gtv:loop=bad
func f(ctx context.Context, xs []int) {
loopLabel:
  for i, x := range xs {
    if x < 0 { goto loopLabel }
    if i > 10 { break loopLabel }
  }
}`
	SetOptions(Options{Level: "regions_logs"})
	out, err := InstrumentProgram([]byte(src), "Demo")
	if err != nil {
		t.Fatalf("instrument error: %v", err)
	}
	s := string(out)
	if strings.Contains(s, "\"loop:bad\"") {
		t.Fatalf("unexpected loop:bad region for loop with goto/labelled break: %s", s)
	}
}

func TestInstrument_ChPtrLoggedBeforeRegion(t *testing.T) {
	const src = `package p
import "context"
func f(ctx context.Context, ch chan int) {
    ch <- 1
    _ = <-ch
}`
	out := instrumentSourceToString(t, src)
	norm := strings.Join(strings.Fields(out), " ")
	logMarker := `trace.Log(ctx, "ch_ptr"`
	if !strings.Contains(norm, logMarker) {
		t.Fatalf("instrumented output missing ch_ptr log: %s", out)
	}
	if strings.Count(norm, logMarker) < 2 {
		t.Fatalf("expected at least two ch_ptr logs, got %d", strings.Count(out, logMarker))
	}
	firstIdx := strings.Index(norm, logMarker)
	withIdx := strings.Index(norm, "trace.WithRegion(ctx")
	if firstIdx < 0 || withIdx < 0 || firstIdx > withIdx {
		t.Fatalf("ch_ptr log should precede trace.WithRegion: %s", out)
	}
}

func TestInstrument_GoSpawnLogsSid(t *testing.T) {
	const src = `package p
import "context"
func f(ctx context.Context) {
    go func() {
        _ = 1
    }()
}`
	out := instrumentSourceToString(t, src)
	if !strings.Contains(out, "var gtvSpawnID uint64") {
		t.Fatalf("expected gtvSpawnID declaration in instrumented output")
	}
	if !strings.Contains(out, "atomic.AddUint64(&gtvSpawnID, 1)") {
		t.Fatalf("expected atomic increment for spawn counter")
	}
	norm := strings.Join(strings.Fields(out), " ")
	parentLog := `trace.Log( ctx, "spawn_parent", fmt.Sprintf("sid=%d", __gtvSpawn1))`
	childLog := `trace.Log( ctx, "spawn_child", fmt.Sprintf("sid=%d", __gtvSpawn1))`
	if !strings.Contains(norm, parentLog) {
		t.Fatalf("missing spawn_parent log; output: %s", out)
	}
	if !strings.Contains(norm, childLog) {
		t.Fatalf("missing spawn_child log; output: %s", out)
	}
	goIdx := strings.Index(norm, "go func(")
	parentIdx := strings.Index(norm, parentLog)
	if goIdx < 0 || parentIdx < 0 || parentIdx > goIdx {
		t.Fatalf("spawn_parent log should appear before go statement")
	}
}

// -------- Golden harness --------

var updateGolden = flag.Bool("update", false, "update golden files")

func loadGolden(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("testdata", name+".golden.go")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("golden %s missing; run with -update to create", path)
		return ""
	}
	return string(b)
}

func writeGolden(t *testing.T, name, content string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir testdata: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write golden: %v", err)
	}
}

func instrumentSourceToString(t *testing.T, src string) string {
	t.Helper()
	SetOptions(Options{Level: "regions_logs"})
	out, err := InstrumentProgram([]byte(src), "Demo")
	if err != nil {
		t.Fatalf("instrument error: %v", err)
	}
	// format the output
	formatted, err := format.Source(out)
	if err != nil {
		return string(out)
	}
	return string(formatted)
}

func TestGolden_JSON(t *testing.T) {
	const src = `package p

import (
  "context"
  "encoding/json"
)

func f(ctx context.Context, v any) ([]byte, error) {
  return json.Marshal(v)
}`
	out := instrumentSourceToString(t, src)
	const name = "json_simple"
	if *updateGolden {
		writeGolden(t, name, out)
	}
	want := loadGolden(t, name)
	if want == "" {
		t.Skip("no golden")
	}
	if out != want {
		t.Fatalf("golden mismatch for %s", name)
	}
}

func TestGolden_DB(t *testing.T) {
	const src = `package p

import (
  "context"
  "database/sql"
)

func f(ctx context.Context, db *sql.DB) error {
  _, err := db.QueryContext(ctx, "SELECT 1")
  return err
}`
	out := instrumentSourceToString(t, src)
	const name = "db_simple"
	if *updateGolden {
		writeGolden(t, name, out)
	}
	want := loadGolden(t, name)
	if want == "" {
		t.Skip("no golden")
	}
	if out != want {
		t.Fatalf("golden mismatch for %s", name)
	}
}

func TestGolden_LoopForced(t *testing.T) {
	const src = `package p
import "context"
// gtv:loop=hot
func f(ctx context.Context, xs []int){
  for _, x := range xs {
    if x%2==0 { continue }
    if x>10 { break }
  }
}`
	out := instrumentSourceToString(t, src)
	const name = "loop_forced"
	if *updateGolden {
		writeGolden(t, name, out)
	}
	want := loadGolden(t, name)
	if want == "" {
		t.Skip("no golden")
	}
	if out != want {
		t.Fatalf("golden mismatch for %s", name)
	}
}

func TestGolden_SelectCases(t *testing.T) {
	const src = `package p

import "context"

func f(ctx context.Context, ch chan int, out int) {
  select {
  case v := <-ch:
    _ = v
  }
  select {
  case ch <- out:
  }
}`
	out := instrumentSourceToString(t, src)
	if !strings.Contains(out, "receive from") {
		t.Fatalf("expected receive region label in output; got:\n%s", out)
	}
	if !strings.Contains(out, "send to") {
		t.Fatalf("expected send region label in output; got:\n%s", out)
	}
	const name = "select_cases"
	if *updateGolden {
		writeGolden(t, name, out)
	}
	want := loadGolden(t, name)
	if want == "" {
		t.Skip("no golden")
	}
	if out != want {
		t.Fatalf("golden mismatch for %s", name)
	}
}
