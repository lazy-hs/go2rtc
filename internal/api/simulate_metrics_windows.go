package api

import (
	"sort"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const simulateProcessQueryAccess = windows.PROCESS_QUERY_LIMITED_INFORMATION | windows.PROCESS_VM_READ

var procGetProcessMemoryInfo = windows.NewLazySystemDLL("psapi.dll").NewProc("GetProcessMemoryInfo")

type processMemoryCounters struct {
	CB                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

func simulateProcessTree(rootPID int) ([]simulateProcessMetric, error) {
	all, err := simulateWindowsProcesses()
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
		enrichWindowsProcess(&process)
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

func simulateWindowsProcesses() ([]simulateProcessMetric, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snapshot)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err = windows.Process32First(snapshot, &entry); err != nil {
		return nil, err
	}

	var processes []simulateProcessMetric
	for {
		processes = append(processes, simulateProcessMetric{
			PID:       int(entry.ProcessID),
			ParentPID: int(entry.ParentProcessID),
			Name:      windows.UTF16ToString(entry.ExeFile[:]),
		})
		if err = windows.Process32Next(snapshot, &entry); err != nil {
			break
		}
	}

	return processes, nil
}

func enrichWindowsProcess(process *simulateProcessMetric) {
	handle, err := windows.OpenProcess(simulateProcessQueryAccess, false, uint32(process.PID))
	if err != nil {
		return
	}
	defer windows.CloseHandle(handle)

	var creation, exit, kernel, user windows.Filetime
	if err = windows.GetProcessTimes(handle, &creation, &exit, &kernel, &user); err == nil {
		process.cpuSeconds = filetimeSeconds(kernel) + filetimeSeconds(user)
	}

	var counters processMemoryCounters
	counters.CB = uint32(unsafe.Sizeof(counters))
	if getProcessMemoryInfo(handle, &counters, counters.CB) {
		process.MemoryBytes = uint64(counters.WorkingSetSize)
	}
}

func getProcessMemoryInfo(handle windows.Handle, counters *processMemoryCounters, size uint32) bool {
	ret, _, _ := procGetProcessMemoryInfo.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(counters)),
		uintptr(size),
	)
	return ret != 0
}

func filetimeSeconds(filetime windows.Filetime) float64 {
	return float64(uint64(filetime.HighDateTime)<<32|uint64(filetime.LowDateTime)) / 1e7
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
