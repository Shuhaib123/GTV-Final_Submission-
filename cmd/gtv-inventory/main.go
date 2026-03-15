package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"

	"jspt/internal/traceproc"
)

type envelope struct {
	Events []traceproc.TimelineEvent `json:"events"`
}

type fieldStat struct {
	name  string
	count int
}

func main() {
	path := flag.String("file", "trace.json", "path to trace.json")
	flag.Parse()

	raw, err := os.ReadFile(*path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n", *path, err)
		os.Exit(1)
	}

	events, err := decodeEvents(raw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse %s: %v\n", *path, err)
		os.Exit(1)
	}
	if len(events) == 0 {
		fmt.Println("no events found")
		return
	}

	counts := make(map[string]int)
	fields := map[string]int{
		"g":           0,
		"time_ms":     0,
		"time_ns":     0,
		"seq":         0,
		"channel":     0,
		"channel_key": 0,
		"ch_ptr":      0,
		"msg_id":      0,
		"attempt_id":  0,
		"select_id":   0,
		"block_kind":  0,
	}

	for _, ev := range events {
		if ev.Event != "" {
			counts[ev.Event]++
		}
		if ev.G != 0 {
			fields["g"]++
		}
		if ev.TimeMs != 0 {
			fields["time_ms"]++
		}
		if ev.TimeNs != 0 {
			fields["time_ns"]++
		}
		if ev.Seq != 0 {
			fields["seq"]++
		}
		if ev.Channel != "" {
			fields["channel"]++
		}
		if ev.ChannelKey != "" {
			fields["channel_key"]++
		}
		if ev.ChPtr != "" {
			fields["ch_ptr"]++
		}
		if ev.MsgID != 0 {
			fields["msg_id"]++
		}
		if ev.AttemptID != "" {
			fields["attempt_id"]++
		}
		if ev.SelectID != "" {
			fields["select_id"]++
		}
		if ev.BlockKind != "" {
			fields["block_kind"]++
		}
	}

	fmt.Printf("events: %d\n", len(events))
	fmt.Println("\nEvent counts:")
	printCounts(counts)

	fmt.Println("\nField presence:")
	stats := make([]fieldStat, 0, len(fields))
	for k, v := range fields {
		stats = append(stats, fieldStat{name: k, count: v})
	}
	sort.Slice(stats, func(i, j int) bool { return stats[i].name < stats[j].name })
	for _, s := range stats {
		fmt.Printf("- %s: %d\n", s.name, s.count)
	}
}

func decodeEvents(raw []byte) ([]traceproc.TimelineEvent, error) {
	var env envelope
	if err := json.Unmarshal(raw, &env); err == nil && len(env.Events) > 0 {
		return env.Events, nil
	}
	var events []traceproc.TimelineEvent
	if err := json.Unmarshal(raw, &events); err != nil {
		return nil, err
	}
	return events, nil
}

func printCounts(counts map[string]int) {
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("- %s: %d\n", k, counts[k])
	}
}
