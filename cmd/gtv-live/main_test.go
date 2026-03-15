package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func boolPtr(v bool) *bool { return &v }

func TestBuildInstrumentOptionsSyncValidationPreset(t *testing.T) {
	req := instrumentReq{
		Level:          "regions",
		BlockRegions:   boolPtr(false),
		GorRegions:     boolPtr(false),
		GuardLabels:    boolPtr(false),
		SyncValidation: boolPtr(true),
	}
	opts, applied := buildInstrumentOptions(req)
	if !applied {
		t.Fatalf("expected sync validation to be applied")
	}
	if opts.Level != "regions_logs" {
		t.Fatalf("level=%q, want regions_logs", opts.Level)
	}
	if !opts.AddBlockRegions {
		t.Fatalf("AddBlockRegions=false, want true")
	}
	if !opts.AddGoroutineRegions {
		t.Fatalf("AddGoroutineRegions=false, want true")
	}
	if !opts.GuardDynamicLabels {
		t.Fatalf("GuardDynamicLabels=false, want true")
	}
}

func TestBuildInstrumentOptionsWithoutSyncValidationKeepsRequest(t *testing.T) {
	req := instrumentReq{
		Level:          "tasks_only",
		BlockRegions:   boolPtr(false),
		GorRegions:     boolPtr(false),
		GuardLabels:    boolPtr(false),
		SyncValidation: boolPtr(false),
	}
	opts, applied := buildInstrumentOptions(req)
	if applied {
		t.Fatalf("sync validation should not be applied")
	}
	if opts.Level != "tasks_only" {
		t.Fatalf("level=%q, want tasks_only", opts.Level)
	}
	if opts.AddBlockRegions {
		t.Fatalf("AddBlockRegions=true, want false")
	}
	if opts.AddGoroutineRegions {
		t.Fatalf("AddGoroutineRegions=true, want false")
	}
	if opts.GuardDynamicLabels {
		t.Fatalf("GuardDynamicLabels=true, want false")
	}
}

func TestListGeneratedWorkloadNames(t *testing.T) {
	dir := t.TempDir()
	mustWrite := func(name string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte("package workload\n"), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	mustWrite("zeta_gen.go")
	mustWrite("Alpha_gen.go")
	mustWrite("not_workload.go")
	mustWrite("bad-name_gen.go")
	mustWrite("_gen.go")

	got, err := listGeneratedWorkloadNames(dir)
	if err != nil {
		t.Fatalf("listGeneratedWorkloadNames error: %v", err)
	}
	want := []string{"alpha", "zeta"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("names=%v, want %v", got, want)
	}
}
