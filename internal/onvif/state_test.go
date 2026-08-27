package onvif

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDisabledONVIFServiceRejectsRequests(t *testing.T) {
	previous := ONVIFEnabled()
	SetONVIFEnabled(false)
	t.Cleanup(func() { SetONVIFEnabled(previous) })

	req := httptest.NewRequest(http.MethodPost, "/onvif/device_service", strings.NewReader("<Envelope/>"))
	res := httptest.NewRecorder()
	onvifDeviceService(res, req)

	require.Equal(t, http.StatusServiceUnavailable, res.Code)
	require.Contains(t, res.Body.String(), "ONVIF service disabled")
}
