//go:build !linux && !windows && !darwin

package api

func simulateProcessTree(rootPID int) ([]simulateProcessMetric, error) {
	return []simulateProcessMetric{simulateFallbackProcess(rootPID)}, nil
}
