package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
	"sort"
	"time"
)

type modeReport struct {
	Mode            string `json:"mode"`
	Cycles          int    `json:"cycles"`
	P50Nanoseconds  int64  `json:"p50_nanoseconds"`
	P95Nanoseconds  int64  `json:"p95_nanoseconds"`
	MaxNanoseconds  int64  `json:"max_nanoseconds"`
	AcceptedAborts  int    `json:"accepted_aborts"`
	CompletedAborts int    `json:"completed_aborts"`
}

type report struct {
	Schema    string       `json:"schema"`
	GoVersion string       `json:"go_version"`
	GOOS      string       `json:"goos"`
	GOARCH    string       `json:"goarch"`
	Modes     []modeReport `json:"modes"`
}

func main() {
	cycles := flag.Int("cycles", 1000, "abort cycles per mode")
	timeout := flag.Duration("timeout", 250*time.Millisecond, "per-cycle completion timeout")
	flag.Parse()
	if *cycles < 1 || *cycles > 10000 {
		fatalf("cycles must be in 1..10000")
	}
	if *timeout < time.Millisecond || *timeout > 5*time.Second {
		fatalf("timeout must be in 1ms..5s")
	}

	result := report{
		Schema:    "go.goroutine-abort.probe.v1",
		GoVersion: runtime.Version(),
		GOOS:      runtime.GOOS,
		GOARCH:    runtime.GOARCH,
		Modes: []modeReport{
			runMode("graceful", runtime.GoroutineAbortGraceful, *cycles, *timeout),
			runMode("hard", runtime.GoroutineAbortHard, *cycles, *timeout),
		},
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(result); err != nil {
		fatalf("encode report: %v", err)
	}
}

func runMode(name string, mode runtime.GoroutineAbortMode, cycles int, timeout time.Duration) modeReport {
	durations := make([]int64, 0, cycles)
	accepted := 0
	completed := 0
	for cycle := 0; cycle < cycles; cycle++ {
		ready := make(chan runtime.GoroutineHandle, 1)
		go func() {
			defer func() {}()
			ready <- runtime.CurrentGoroutine()
			for {
			}
		}()
		handle := <-ready
		started := time.Now()
		if handle.Abort(mode) {
			accepted++
		}
		deadline := time.Now().Add(timeout)
		for handle.Valid() && time.Now().Before(deadline) {
			runtime.Gosched()
		}
		if !handle.Valid() {
			completed++
			durations = append(durations, time.Since(started).Nanoseconds())
		}
	}
	if len(durations) == 0 {
		return modeReport{Mode: name, Cycles: cycles, AcceptedAborts: accepted}
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	return modeReport{
		Mode:            name,
		Cycles:          cycles,
		P50Nanoseconds:  percentile(durations, 50),
		P95Nanoseconds:  percentile(durations, 95),
		MaxNanoseconds:  durations[len(durations)-1],
		AcceptedAborts:  accepted,
		CompletedAborts: completed,
	}
}

func percentile(sorted []int64, percentage int) int64 {
	index := (percentage*len(sorted) + 99) / 100
	if index < 1 {
		index = 1
	}
	return sorted[index-1]
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(2)
}
