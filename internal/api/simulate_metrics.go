package api

import (
	"net/http"
	"os"
	"runtime"
	"runtime/metrics"
	"sync"
	"time"
)

var simulateStartedAt = time.Now()

type simulateMetricsInfo struct {
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryAlloc   uint64  `json:"memory_alloc"`
	MemorySys     uint64  `json:"memory_sys"`
	NumGC         uint32  `json:"num_gc"`
	Goroutines    int     `json:"goroutines"`
	UptimeSeconds int64   `json:"uptime_seconds"`
	ProcessID     int     `json:"process_id"`
	Timestamp     string  `json:"timestamp"`
}

var simulateCPUSnapshot = struct {
	sync.Mutex
	seconds float64
	at      time.Time
}{}

func simulateMetricsHandler(w http.ResponseWriter, _ *http.Request) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	ResponseJSON(w, &simulateMetricsInfo{
		CPUPercent:    simulateCPUPercent(),
		MemoryAlloc:   mem.Alloc,
		MemorySys:     mem.Sys,
		NumGC:         mem.NumGC,
		Goroutines:    runtime.NumGoroutine(),
		UptimeSeconds: int64(time.Since(simulateStartedAt).Seconds()),
		ProcessID:     os.Getpid(),
		Timestamp:     time.Now().Format(time.RFC3339),
	})
}

func simulateCPUPercent() float64 {
	samples := []metrics.Sample{
		{Name: "/cpu/classes/user:cpu-seconds"},
		{Name: "/cpu/classes/gc/total:cpu-seconds"},
		{Name: "/cpu/classes/scavenge/total:cpu-seconds"},
	}
	metrics.Read(samples)

	var seconds float64
	for _, sample := range samples {
		if sample.Value.Kind() == metrics.KindFloat64 {
			seconds += sample.Value.Float64()
		}
	}

	now := time.Now()
	simulateCPUSnapshot.Lock()
	defer simulateCPUSnapshot.Unlock()

	if simulateCPUSnapshot.at.IsZero() {
		simulateCPUSnapshot.seconds = seconds
		simulateCPUSnapshot.at = now
		return 0
	}

	elapsed := now.Sub(simulateCPUSnapshot.at).Seconds()
	used := seconds - simulateCPUSnapshot.seconds
	simulateCPUSnapshot.seconds = seconds
	simulateCPUSnapshot.at = now
	if elapsed <= 0 || used < 0 {
		return 0
	}

	return used / elapsed / float64(runtime.NumCPU()) * 100
}
