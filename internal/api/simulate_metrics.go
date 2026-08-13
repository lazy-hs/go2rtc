package api

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

var simulateStartedAt = time.Now()

type simulateMetricsInfo struct {
	CPUPercent    float64                 `json:"cpu_percent"`
	MemoryAlloc   uint64                  `json:"memory_alloc"`
	MemorySys     uint64                  `json:"memory_sys"`
	GoMemoryAlloc uint64                  `json:"go_memory_alloc"`
	GoMemorySys   uint64                  `json:"go_memory_sys"`
	NumGC         uint32                  `json:"num_gc"`
	Goroutines    int                     `json:"goroutines"`
	UptimeSeconds int64                   `json:"uptime_seconds"`
	ProcessID     int                     `json:"process_id"`
	ProcessCount  int                     `json:"process_count"`
	Processes     []simulateProcessMetric `json:"processes"`
	Timestamp     string                  `json:"timestamp"`
}

type simulateProcessMetric struct {
	PID         int     `json:"pid"`
	ParentPID   int     `json:"parent_pid,omitempty"`
	Name        string  `json:"name"`
	Role        string  `json:"role,omitempty"`
	Source      string  `json:"source,omitempty"`
	CPUPercent  float64 `json:"cpu_percent"`
	MemoryBytes uint64  `json:"memory_bytes"`
	cpuSeconds  float64
}

type simulateProcessSourceInfo struct {
	Source string
}

var simulateProcessCPUSnapshot = struct {
	sync.Mutex
	seconds map[int]float64
	at      time.Time
}{
	seconds: make(map[int]float64),
}

var simulateProcessSources = struct {
	sync.RWMutex
	byPID map[int]simulateProcessSourceInfo
}{
	byPID: make(map[int]simulateProcessSourceInfo),
}

func simulateMetricsHandler(w http.ResponseWriter, _ *http.Request) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	now := time.Now()
	processes, _ := simulateProcessTree(os.Getpid())
	enrichSimulateProcessSources(processes)
	cpuPercent := simulateProcessCPUPercent(processes, now)
	memoryAlloc := simulateProcessMemory(processes)
	if memoryAlloc == 0 {
		memoryAlloc = mem.Alloc
	}

	ResponseJSON(w, &simulateMetricsInfo{
		CPUPercent:    cpuPercent,
		MemoryAlloc:   memoryAlloc,
		MemorySys:     mem.Sys,
		GoMemoryAlloc: mem.Alloc,
		GoMemorySys:   mem.Sys,
		NumGC:         mem.NumGC,
		Goroutines:    runtime.NumGoroutine(),
		UptimeSeconds: int64(time.Since(simulateStartedAt).Seconds()),
		ProcessID:     os.Getpid(),
		ProcessCount:  len(processes),
		Processes:     processes,
		Timestamp:     now.Format(time.RFC3339),
	})
}

func RegisterSimulateProcess(pid int, source string) {
	if pid <= 0 {
		return
	}

	simulateProcessSources.Lock()
	simulateProcessSources.byPID[pid] = simulateProcessSourceInfo{
		Source: source,
	}
	simulateProcessSources.Unlock()
}

func UnregisterSimulateProcess(pid int) {
	if pid <= 0 {
		return
	}

	simulateProcessSources.Lock()
	delete(simulateProcessSources.byPID, pid)
	simulateProcessSources.Unlock()
}

func enrichSimulateProcessSources(processes []simulateProcessMetric) {
	simulateProcessSources.RLock()
	sources := make(map[int]simulateProcessSourceInfo, len(simulateProcessSources.byPID))
	for pid, info := range simulateProcessSources.byPID {
		sources[pid] = info
	}
	simulateProcessSources.RUnlock()

	parentByPID := make(map[int]int, len(processes))
	for _, process := range processes {
		parentByPID[process.PID] = process.ParentPID
	}

	for i := range processes {
		info, ok := sourceForSimulateProcess(processes[i].PID, sources, parentByPID)
		if !ok {
			continue
		}
		processes[i].Source = info.Source
	}
}

func sourceForSimulateProcess(pid int, sources map[int]simulateProcessSourceInfo, parentByPID map[int]int) (simulateProcessSourceInfo, bool) {
	seen := map[int]struct{}{}
	for pid > 0 {
		if info, ok := sources[pid]; ok {
			return info, true
		}
		if _, ok := seen[pid]; ok {
			break
		}
		seen[pid] = struct{}{}
		pid = parentByPID[pid]
	}
	return simulateProcessSourceInfo{}, false
}

func simulateProcessCPUPercent(processes []simulateProcessMetric, now time.Time) float64 {
	simulateProcessCPUSnapshot.Lock()
	defer simulateProcessCPUSnapshot.Unlock()

	current := make(map[int]float64, len(processes))
	for _, process := range processes {
		current[process.PID] = process.cpuSeconds
	}

	if simulateProcessCPUSnapshot.at.IsZero() {
		simulateProcessCPUSnapshot.seconds = current
		simulateProcessCPUSnapshot.at = now
		return 0
	}

	elapsed := now.Sub(simulateProcessCPUSnapshot.at).Seconds()
	if elapsed <= 0 {
		simulateProcessCPUSnapshot.seconds = current
		simulateProcessCPUSnapshot.at = now
		return 0
	}

	var total float64
	for i := range processes {
		used := processes[i].cpuSeconds - simulateProcessCPUSnapshot.seconds[processes[i].PID]
		if used < 0 {
			used = 0
		}
		processes[i].CPUPercent = used / elapsed / float64(runtime.NumCPU()) * 100
		total += processes[i].CPUPercent
	}

	simulateProcessCPUSnapshot.seconds = current
	simulateProcessCPUSnapshot.at = now
	return total
}

func simulateProcessMemory(processes []simulateProcessMetric) uint64 {
	var total uint64
	for _, process := range processes {
		total += process.MemoryBytes
	}
	return total
}

func simulateFallbackProcess(pid int) simulateProcessMetric {
	name := "go2rtc"
	if path, err := os.Executable(); err == nil && path != "" {
		name = filepath.Base(path)
	}
	return simulateProcessMetric{
		PID:  pid,
		Name: name,
		Role: "go2rtc",
	}
}
