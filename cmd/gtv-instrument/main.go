package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"jspt/internal/instrumenter"
)

func main() {
	in := flag.String("in", "", "input Go file (main program)")
	name := flag.String("name", "Demo", "workload name (e.g., MyProg)")
	outDir := flag.String("outdir", "internal/workload", "output directory for generated workload file")
	mvpDefault := envBool("GTV_MVP")
	mvp := flag.Bool("mvp", mvpDefault, "force MVP defaults (disable IO/loop/HTTP/GRPC/value logs)")
	// Instrumenter options
	guard := flag.Bool("guard-labels", true, "guard dynamic labels with trace.IsEnabled")
	gorRegions := flag.Bool("goroutine-regions", false, "add goroutine:<name> regions at go sites")
	blockRegions := flag.Bool("block-regions", false, "wrap wg.Wait/mutex.* and related in regions")
	ioRegions := flag.Bool("io-regions", false, "wrap common I/O (http/os/io) in regions")
	ioJSON := flag.Bool("io-json", false, "wrap encoding/json calls in regions")
	ioDB := flag.Bool("io-db", false, "wrap database/sql calls in regions")
	level := flag.String("level", "regions", "instrumentation level: tasks_only|regions|regions_logs")
	grpcTasks := flag.Bool("grpc-tasks", false, "add tasks to gRPC handlers (methods)")
	httpTasks := flag.Bool("http-tasks", false, "add tasks to HTTP handlers")
	loopRegions := flag.Bool("loop-regions", false, "wrap safe loop bodies in regions")
	syncValidation := flag.Bool("sync-validation", envBool("GTV_SYNC_VALIDATION"), "enforce sync-validation instrumentation preset (regions_logs + block/goroutine regions)")
	valueLogs := flag.Bool("value-logs", instrumenter.ValueLogsEnv(false), "emit trace.Log(ctx,\"value\",...) annotations when regions_logs enabled")
	includePkgs := flag.String("include-pkgs", "", "comma or pipe separated package patterns to include (others downgraded to tasks_only)")
	excludePkgs := flag.String("exclude-pkgs", "", "comma or pipe separated package patterns to exclude (downgraded to tasks_only)")
	flag.Parse()
	if *in == "" {
		log.Fatal("-in is required")
	}
	if *mvp {
		_ = os.Setenv("GTV_MVP", "1")
	}
	if *syncValidation {
		*level = "regions_logs"
		*blockRegions = true
		*gorRegions = true
		*guard = true
	}
	split := func(s string) []string {
		if s == "" {
			return nil
		}
		f := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '|' })
		out := make([]string, 0, len(f))
		for _, x := range f {
			x = strings.TrimSpace(x)
			if x != "" {
				out = append(out, x)
			}
		}
		return out
	}
	instrumenter.SetOptions(instrumenter.Options{GuardDynamicLabels: *guard, AddGoroutineRegions: *gorRegions, AddBlockRegions: *blockRegions, AddIORegions: *ioRegions, AddIOJSONRegions: *ioJSON, AddIODBRegions: *ioDB, AddGRPCTasks: *grpcTasks, AddHTTPHandlerTasks: *httpTasks, AddLoopRegions: *loopRegions, AddValueLogs: *valueLogs, IncludePackages: split(*includePkgs), ExcludePackages: split(*excludePkgs), Level: *level})
	src, err := ioutil.ReadFile(*in)
	if err != nil {
		log.Fatal(err)
	}
	code, err := instrumenter.InstrumentProgram(src, *name)
	if err != nil {
		log.Fatal(err)
	}
	base := strings.ToLower(*name)
	outPath := filepath.Join(*outDir, base+"_gen.go")
	tag := []byte(fmt.Sprintf("//go:build workload_%s\n// +build workload_%s\n\n", base, base))
	if err := ioutil.WriteFile(outPath, append(tag, code...), 0644); err != nil {
		log.Fatal(err)
	}
	fmt.Println("wrote", outPath)
	fmt.Println("You can now run: -workload=", strings.ToLower(*name))
}

func envBool(name string) bool {
	v := os.Getenv(name)
	return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
}
