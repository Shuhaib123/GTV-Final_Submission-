package instrumenter

import (
	"encoding/json"
	"go/ast"
	"os"
	"path"
	"strings"
	"time"
)

// Options is the effective configuration used by the instrumenter.
type Options struct {
	Level               string
	GuardDynamicLabels  bool
	AddGoroutineRegions bool
	AddBlockRegions     bool
	AddHTTPHandlerTasks bool
	AddGRPCTasks        bool
	AddLoopRegions      bool

	AddIORegions       bool
	AddIOJSONRegions   bool
	AddIODBRegions     bool
	AddIOHTTPRegions   bool
	AddIOOSRegions     bool
	IOAssumeBackground bool

	IncludePackages []string
	ExcludePackages []string
}

// Default options (you can tune these to your current defaults)
var instrOpts = Options{
	Level:               "",
	GuardDynamicLabels:  true,
	AddGoroutineRegions: true,
	AddBlockRegions:     true,
	AddHTTPHandlerTasks: false,
	AddGRPCTasks:        false,
	AddLoopRegions:      false,

	AddIORegions:       false,
	AddIOJSONRegions:   false,
	AddIODBRegions:     false,
	AddIOHTTPRegions:   false,
	AddIOOSRegions:     false,
	IOAssumeBackground: false,
}

// SetOptions overrides the package-level default options at runtime. Tests
// and callers can use this to programmatically change instrumentation flags.
func SetOptions(o Options) {
	instrOpts = o
}

func boolEnv(name string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return def
	}
	v = strings.ToLower(v)
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

// ParseOptionsFromEnvAndDirectives merges defaults, env, optional JSON config,
// package include/exclude, and top-of-file // gtv:* directives.
//
// Returns: effective options, onlySet/skipSet filters, (timeoutNS, hasTimeout, doneName)
func ParseOptionsFromEnvAndDirectives(file *ast.File, base Options) (opts Options, onlySet map[string]bool, skipSet map[string]bool, timeoutNS int64, hasTimeout bool, doneName string) {
	opts = base
	onlySet = make(map[string]bool)
	skipSet = make(map[string]bool)

	// ---- Env merges ----
	opts.GuardDynamicLabels = boolEnv("GTV_INSTR_GUARD_LABELS", opts.GuardDynamicLabels)
	opts.AddGoroutineRegions = boolEnv("GTV_INSTR_GOROUTINE_REGIONS", opts.AddGoroutineRegions)
	opts.AddBlockRegions = boolEnv("GTV_INSTR_BLOCK_REGIONS", opts.AddBlockRegions)
	opts.AddHTTPHandlerTasks = boolEnv("GTV_INSTR_HTTP_TASKS", opts.AddHTTPHandlerTasks)
	opts.AddIORegions = boolEnv("GTV_INSTR_IO_REGIONS", opts.AddIORegions)
	opts.AddIOJSONRegions = boolEnv("GTV_INSTR_IO_JSON", opts.AddIOJSONRegions)
	opts.AddIODBRegions = boolEnv("GTV_INSTR_IO_DB", opts.AddIODBRegions)
	opts.AddIOHTTPRegions = boolEnv("GTV_INSTR_IO_HTTP", opts.AddIOHTTPRegions)
	opts.AddIOOSRegions = boolEnv("GTV_INSTR_IO_OS", opts.AddIOOSRegions)
	opts.IOAssumeBackground = boolEnv("GTV_INSTR_IO_ASSUME_BG", opts.IOAssumeBackground)
	opts.AddGRPCTasks = boolEnv("GTV_INSTR_GRPC_TASKS", opts.AddGRPCTasks)
	opts.AddLoopRegions = boolEnv("GTV_INSTR_LOOP_REGIONS", opts.AddLoopRegions)
	if lvl := strings.TrimSpace(os.Getenv("GTV_INSTR_LEVEL")); lvl != "" {
		opts.Level = strings.ToLower(lvl)
	}

	// ---- Optional JSON config (env: GTV_INSTR_CONFIG=path) ----
	if cfgPath := strings.TrimSpace(os.Getenv("GTV_INSTR_CONFIG")); cfgPath != "" {
		type ioConfig struct {
			DatabaseSQL  *bool `json:"database_sql"`
			EncodingJSON *bool `json:"encoding_json"`
			NetHTTP      *bool `json:"net_http"`
			OSIO         *bool `json:"os_io"`
			AssumeBG     *bool `json:"assume_background"`
		}
		type config struct {
			GuardLabels      *bool    `json:"guard_labels"`
			GoroutineRegions *bool    `json:"goroutine_regions"`
			BlockRegions     *bool    `json:"block_regions"`
			HTTPTasks        *bool    `json:"http_tasks"`
			GRPCTasks        *bool    `json:"grpc_tasks"`
			LoopRegions      *bool    `json:"loop_regions"`
			IORegions        *bool    `json:"io_regions"`
			IO               ioConfig `json:"io"`
			IOAssumeBG       *bool    `json:"io_assume_background"`
			Level            string   `json:"level"`
			Only             []string `json:"only"`
			Skip             []string `json:"skip"`
			IncludePackages  []string `json:"include_packages"`
			ExcludePackages  []string `json:"exclude_packages"`
		}
		if b, err := os.ReadFile(cfgPath); err == nil {
			var c config
			if json.Unmarshal(b, &c) == nil {
				if c.GuardLabels != nil {
					opts.GuardDynamicLabels = *c.GuardLabels
				}
				if c.GoroutineRegions != nil {
					opts.AddGoroutineRegions = *c.GoroutineRegions
				}
				if c.BlockRegions != nil {
					opts.AddBlockRegions = *c.BlockRegions
				}
				if c.HTTPTasks != nil {
					opts.AddHTTPHandlerTasks = *c.HTTPTasks
				}
				if c.GRPCTasks != nil {
					opts.AddGRPCTasks = *c.GRPCTasks
				}
				if c.LoopRegions != nil {
					opts.AddLoopRegions = *c.LoopRegions
				}
				if c.IORegions != nil {
					opts.AddIORegions = *c.IORegions
				}
				if c.IO.DatabaseSQL != nil {
					opts.AddIODBRegions = *c.IO.DatabaseSQL
				}
				if c.IO.EncodingJSON != nil {
					opts.AddIOJSONRegions = *c.IO.EncodingJSON
				}
				if c.IO.NetHTTP != nil {
					opts.AddIOHTTPRegions = *c.IO.NetHTTP
				}
				if c.IO.OSIO != nil {
					opts.AddIOOSRegions = *c.IO.OSIO
				}
				if c.IO.AssumeBG != nil {
					opts.IOAssumeBackground = *c.IO.AssumeBG
				}
				if c.IOAssumeBG != nil {
					opts.IOAssumeBackground = *c.IOAssumeBG
				}
				if c.Level != "" {
					opts.Level = strings.ToLower(c.Level)
				}
				for _, s := range c.Only {
					s = strings.TrimSpace(s)
					if s != "" {
						onlySet[s] = true
					}
				}
				for _, s := range c.Skip {
					s = strings.TrimSpace(s)
					if s != "" {
						skipSet[s] = true
					}
				}
				if len(c.IncludePackages) > 0 {
					opts.IncludePackages = append([]string{}, c.IncludePackages...)
				}
				if len(c.ExcludePackages) > 0 {
					opts.ExcludePackages = append([]string{}, c.ExcludePackages...)
				}
				// Package gating -> downgrade to tasks_only if excluded
				pkgName := file.Name.Name
				matchAny := func(name string, list []string) bool {
					for _, p := range list {
						p = strings.TrimSpace(p)
						if p == "" {
							continue
						}
						if strings.ContainsAny(p, "*?[") {
							if ok, _ := path.Match(p, name); ok {
								return true
							}
						} else if strings.EqualFold(p, name) {
							return true
						}
					}
					return false
				}
				if len(c.IncludePackages) > 0 && !matchAny(pkgName, c.IncludePackages) {
					opts = downgradeToTasksOnly(opts)
				}
				if matchAny(pkgName, c.ExcludePackages) {
					opts = downgradeToTasksOnly(opts)
				}
			}
		}
	}

	// Also apply include/exclude directly from opts, if any
	if len(opts.IncludePackages) > 0 || len(opts.ExcludePackages) > 0 {
		pkgName := file.Name.Name
		matchAny := func(name string, list []string) bool {
			for _, p := range list {
				p = strings.TrimSpace(p)
				if p == "" {
					continue
				}
				if strings.ContainsAny(p, "*?[") {
					if ok, _ := path.Match(p, name); ok {
						return true
					}
				} else if strings.EqualFold(p, name) {
					return true
				}
			}
			return false
		}
		if (len(opts.IncludePackages) > 0 && !matchAny(pkgName, opts.IncludePackages)) || matchAny(pkgName, opts.ExcludePackages) {
			opts = downgradeToTasksOnly(opts)
		}
	}

	// ---- Top-of-file // gtv:* directives ----
	for _, cg := range file.Comments {
		for _, c := range cg.List {
			txt := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
			if !strings.HasPrefix(txt, "gtv:") {
				continue
			}
			body := strings.TrimSpace(strings.TrimPrefix(txt, "gtv:"))
			parts := strings.Split(body, ",")
			for _, part := range parts {
				kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
				if len(kv) != 2 {
					continue
				}
				key := strings.ToLower(strings.TrimSpace(kv[0]))
				val := strings.TrimSpace(kv[1])
				switch key {
				case "timeout", "bounded":
					if d, err := time.ParseDuration(val); err == nil {
						timeoutNS = d.Nanoseconds()
						hasTimeout = true
					}
				case "done":
					if val != "" {
						doneName = strings.Trim(val, "`\"")
					}
				case "goroutine_regions":
					opts.AddGoroutineRegions = parseBoolWord(val, opts.AddGoroutineRegions)
				case "guard_labels":
					opts.GuardDynamicLabels = parseBoolWord(val, opts.GuardDynamicLabels)
				case "level":
					if val != "" {
						opts.Level = strings.ToLower(strings.Trim(val, "`\""))
					}
				case "block_regions":
					opts.AddBlockRegions = parseBoolWord(val, opts.AddBlockRegions)
				case "http_tasks":
					opts.AddHTTPHandlerTasks = parseBoolWord(val, opts.AddHTTPHandlerTasks)
				case "grpc_tasks":
					opts.AddGRPCTasks = parseBoolWord(val, opts.AddGRPCTasks)
				case "loop_regions":
					opts.AddLoopRegions = parseBoolWord(val, opts.AddLoopRegions)
				case "io_regions":
					opts.AddIORegions = parseBoolWord(val, opts.AddIORegions)
				case "only":
					for _, nm := range splitList(val) {
						onlySet[nm] = true
					}
				case "skip":
					for _, nm := range splitList(val) {
						skipSet[nm] = true
					}
				}
			}
		}
	}

	return opts, onlySet, skipSet, timeoutNS, hasTimeout, doneName
}

func downgradeToTasksOnly(o Options) Options {
	o.Level = "tasks_only"
	o.AddGoroutineRegions = false
	o.AddBlockRegions = false
	o.AddHTTPHandlerTasks = false
	o.AddGRPCTasks = false
	o.AddIORegions = false
	o.AddIOJSONRegions = false
	o.AddIODBRegions = false
	o.AddIOHTTPRegions = false
	o.AddIOOSRegions = false
	o.IOAssumeBackground = false
	return o
}

func parseBoolWord(v string, def bool) bool {
	v = strings.ToLower(strings.Trim(v, "`\""))
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}

func splitList(s string) []string {
	s = strings.Trim(s, "`\"")
	if s == "" {
		return nil
	}
	f := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '|' || r == ' ' || r == '\t' })
	out := make([]string, 0, len(f))
	for _, x := range f {
		x = strings.TrimSpace(x)
		if x != "" {
			out = append(out, x)
		}
	}
	return out
}
