package onvif

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/stretchr/testify/require"
)

func TestSimulatePTZAPI(t *testing.T) {
	previous := ptz
	previousConfigPath := app.ConfigPath
	ptz = newPTZManager(map[string]ptzStreamConfig{"main": {}})
	app.ConfigPath = ""
	t.Cleanup(func() {
		ptz = previous
		app.ConfigPath = previousConfigPath
		applyPTZGlobalEnabled(previous.globalEnabled())
	})

	body := bytes.NewBufferString(`{"action":"absolute","pan":0.4,"tilt":-0.2,"zoom":0.6}`)
	request := httptest.NewRequest(http.MethodPut, "/api/simulate/ptz?src=main", body)
	response := httptest.NewRecorder()
	apiSimulatePTZ(response, request)
	require.Equal(t, http.StatusOK, response.Code)

	var status simulatePTZStatus
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &status))
	require.Equal(t, "main", status.Source)
	require.InDelta(t, 0.4, status.Pan, 0.0001)
	require.InDelta(t, -0.2, status.Tilt, 0.0001)
	require.InDelta(t, 0.6, status.Zoom, 0.0001)
	require.Equal(t, 4.0, status.MaxZoom)

	request = httptest.NewRequest(http.MethodGet, "/api/simulate/ptz?src=main", nil)
	response = httptest.NewRecorder()
	apiSimulatePTZ(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), `"source":"main"`)

	request = httptest.NewRequest(http.MethodPut, "/api/simulate/ptz", bytes.NewBufferString(`{"enabled":false}`))
	response = httptest.NewRecorder()
	apiSimulatePTZ(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	require.JSONEq(t, `{"enabled":false}`, response.Body.String())
	require.False(t, ptz.globalEnabled())

	request = httptest.NewRequest(http.MethodGet, "/api/simulate/ptz?global=1", nil)
	response = httptest.NewRecorder()
	apiSimulatePTZ(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	require.JSONEq(t, `{"enabled":false}`, response.Body.String())

	request = httptest.NewRequest(http.MethodPut, "/api/simulate/ptz", bytes.NewBufferString(`{"enabled":true}`))
	response = httptest.NewRecorder()
	apiSimulatePTZ(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	require.True(t, ptz.globalEnabled())
}
