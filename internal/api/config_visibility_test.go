package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfigHandlerDisabled(t *testing.T) {
	previous := configEnabled
	t.Cleanup(func() { configEnabled = previous })
	configEnabled = false

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	res := httptest.NewRecorder()
	configHandlerWithVisibility(res, req)

	require.Equal(t, http.StatusGone, res.Code)
	require.Contains(t, res.Body.String(), "config page disabled")
}
