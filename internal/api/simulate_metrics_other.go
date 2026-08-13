//go:build !linux && !windows

package api

func simulateProcessTree(rootPID int) ([]simulateProcessMetric, error) {
	return []simulateProcessMetric{simulateFallbackProcess(rootPID)}, nil
}
