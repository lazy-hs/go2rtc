package api

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSimulateMetricsHandler(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/simulate/metrics", nil)
	res := httptest.NewRecorder()
	simulateMetricsHandler(res, req)

	require.Equal(t, 200, res.Code)
	var info simulateMetricsInfo
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &info))
	require.Greater(t, info.MemoryAlloc, uint64(0))
	require.Greater(t, info.MemorySys, uint64(0))
	require.Greater(t, info.Goroutines, 0)
	require.Greater(t, info.ProcessID, 0)
	require.NotEmpty(t, info.Timestamp)
	require.GreaterOrEqual(t, info.CPUPercent, 0.0)
}
