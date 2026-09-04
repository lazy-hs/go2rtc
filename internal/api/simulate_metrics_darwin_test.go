//go:build darwin

package api

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseDarwinCPUTime(t *testing.T) {
	tests := map[string]float64{
		"0:01.50":       1.5,
		"1:02:03.25":    3723.25,
		"2-03:04:05.5":  183845.5,
		"10-00:00:00.0": 864000,
	}

	for value, want := range tests {
		got, err := parseDarwinCPUTime(value)
		require.NoError(t, err)
		require.Equal(t, want, got)
	}
}

func TestParseDarwinProcessLine(t *testing.T) {
	process, err := parseDarwinProcessLine("  123  1  1:02:03.25  4096 ffmpeg")
	require.NoError(t, err)
	require.Equal(t, 123, process.PID)
	require.Equal(t, 1, process.ParentPID)
	require.Equal(t, "ffmpeg", process.Name)
	require.Equal(t, uint64(4194304), process.MemoryBytes)
	require.Equal(t, 3723.25, process.cpuSeconds)
}
