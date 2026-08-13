package api

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func simulateProcessTree(rootPID int) ([]simulateProcessMetric, error) {
	all, err := simulateLinuxProcesses()
	if err != nil {
		return []simulateProcessMetric{simulateFallbackProcess(rootPID)}, err
	}

	wanted := map[int]struct{}{rootPID: {}}
	for {
		changed := false
		for _, process := range all {
			if _, ok := wanted[process.ParentPID]; ok {
				if _, exists := wanted[process.PID]; !exists {
					wanted[process.PID] = struct{}{}
					changed = true
				}
			}
		}
		if !changed {
			break
		}
	}

	processes := make([]simulateProcessMetric, 0, len(wanted))
	for _, process := range all {
		if _, ok := wanted[process.PID]; !ok {
			continue
		}
		process.Role = simulateProcessRole(process.PID, rootPID, process.Name)
		processes = append(processes, process)
	}

	sort.Slice(processes, func(i, j int) bool {
		if processes[i].PID == rootPID {
			return true
		}
		if processes[j].PID == rootPID {
			return false
		}
		if processes[i].MemoryBytes != processes[j].MemoryBytes {
			return processes[i].MemoryBytes > processes[j].MemoryBytes
		}
		return processes[i].PID < processes[j].PID
	})

	if len(processes) == 0 {
		processes = []simulateProcessMetric{simulateFallbackProcess(rootPID)}
	}

	return processes, nil
}

func simulateLinuxProcesses() ([]simulateProcessMetric, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}

	clockTicks := simulateClockTicks()
	pageSize := uint64(os.Getpagesize())
	processes := make([]simulateProcessMetric, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		process, err := simulateReadLinuxStat(pid, clockTicks, pageSize)
		if err == nil {
			processes = append(processes, process)
		}
	}

	return processes, nil
}

func simulateReadLinuxStat(pid int, clockTicks float64, pageSize uint64) (simulateProcessMetric, error) {
	statPath := filepath.Join("/proc", strconv.Itoa(pid), "stat")
	data, err := os.ReadFile(statPath)
	if err != nil {
		return simulateProcessMetric{}, err
	}

	text := string(data)
	open := strings.IndexByte(text, '(')
	close := strings.LastIndexByte(text, ')')
	if open < 0 || close <= open {
		return simulateProcessMetric{}, errors.New("invalid proc stat")
	}

	fields := strings.Fields(text[close+2:])
	if len(fields) < 22 {
		return simulateProcessMetric{}, errors.New("short proc stat")
	}

	parentPID, _ := strconv.Atoi(fields[1])
	userTicks, _ := strconv.ParseUint(fields[11], 10, 64)
	systemTicks, _ := strconv.ParseUint(fields[12], 10, 64)
	residentPages, _ := strconv.ParseUint(fields[21], 10, 64)

	name := text[open+1 : close]
	if comm, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "comm")); err == nil {
		if s := strings.TrimSpace(string(comm)); s != "" {
			name = s
		}
	}

	return simulateProcessMetric{
		PID:         pid,
		ParentPID:   parentPID,
		Name:        name,
		MemoryBytes: residentPages * pageSize,
		cpuSeconds:  float64(userTicks+systemTicks) / clockTicks,
	}, nil
}

func simulateClockTicks() float64 {
	// Linux exposes process CPU time in USER_HZ ticks. Most amd64/arm64 systems use 100.
	return 100
}

func simulateProcessRole(pid, rootPID int, name string) string {
	if pid == rootPID {
		return "go2rtc"
	}
	if strings.Contains(strings.ToLower(name), "ffmpeg") {
		return "ffmpeg"
	}
	return "子进程"
}
