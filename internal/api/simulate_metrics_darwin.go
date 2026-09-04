//go:build darwin

package api

import (
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

func simulateProcessTree(rootPID int) ([]simulateProcessMetric, error) {
	all, err := simulateDarwinProcesses()
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

func simulateDarwinProcesses() ([]simulateProcessMetric, error) {
	cmd := exec.Command("/bin/ps", "-axo", "pid=,ppid=,time=,rss=,comm=")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	processes := make([]simulateProcessMetric, 0, len(lines))
	for _, line := range lines {
		process, err := parseDarwinProcessLine(line)
		if err != nil {
			continue
		}
		processes = append(processes, process)
	}

	return processes, nil
}

func parseDarwinProcessLine(line string) (simulateProcessMetric, error) {
	fields := strings.Fields(line)
	if len(fields) < 5 {
		return simulateProcessMetric{}, fmt.Errorf("short ps line: %q", line)
	}

	pid, err := strconv.Atoi(fields[0])
	if err != nil {
		return simulateProcessMetric{}, err
	}
	parentPID, err := strconv.Atoi(fields[1])
	if err != nil {
		return simulateProcessMetric{}, err
	}
	cpuSeconds, err := parseDarwinCPUTime(fields[2])
	if err != nil {
		return simulateProcessMetric{}, err
	}
	rssKB, err := strconv.ParseUint(fields[3], 10, 64)
	if err != nil {
		return simulateProcessMetric{}, err
	}

	name := strings.Join(fields[4:], " ")

	return simulateProcessMetric{
		PID:         pid,
		ParentPID:   parentPID,
		Name:        name,
		MemoryBytes: rssKB * 1024,
		cpuSeconds:  cpuSeconds,
	}, nil
}

func parseDarwinCPUTime(value string) (float64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("empty cpu time")
	}

	var days float64
	if before, after, ok := strings.Cut(value, "-"); ok {
		n, err := strconv.ParseFloat(before, 64)
		if err != nil {
			return 0, err
		}
		days = n
		value = after
	}

	parts := strings.Split(value, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, fmt.Errorf("invalid cpu time: %q", value)
	}

	seconds, err := strconv.ParseFloat(parts[len(parts)-1], 64)
	if err != nil {
		return 0, err
	}
	minutes, err := strconv.ParseFloat(parts[len(parts)-2], 64)
	if err != nil {
		return 0, err
	}

	hours := 0.0
	if len(parts) == 3 {
		hours, err = strconv.ParseFloat(parts[0], 64)
		if err != nil {
			return 0, err
		}
	}

	return days*86400 + hours*3600 + minutes*60 + seconds, nil
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
