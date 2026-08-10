package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestLogConfigHandlerGet(t *testing.T) {
	configPath := setupLogConfigTest(t, `log:
  level: warn
  onvif: debug
  exec: error
  rtsp: trace
`)

	req := httptest.NewRequest(http.MethodGet, "/api/log/config", nil)
	res := httptest.NewRecorder()
	logConfigHandler(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	var response logConfigResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &response))
	require.Equal(t, "warn", response.Config.Level)
	require.Equal(t, map[string]string{
		"onvif": "debug",
		"exec":  "error",
		"rtsp":  "trace",
	}, response.Config.Modules)
	require.Contains(t, response.Modules, "onvif")
	require.Contains(t, response.Modules, "exec")
	require.Contains(t, response.Modules, "rtsp")
	require.Contains(t, response.Levels, "disabled")
	require.FileExists(t, configPath)
}

func TestLogConfigHandlerPutPreservesOtherSettings(t *testing.T) {
	configPath := setupLogConfigTest(t, `# root comment
log:
  level: warn
  format: text
  output: file:go2rtc.log
  time: UNIXMS
  onvif: info
  exec: error
`)

	body := bytes.NewBufferString(`{"level":"INFO","modules":{"onvif":"DEBUG","exec":"","rtsp":"TRACE"}}`)
	req := httptest.NewRequest(http.MethodPut, "/api/log/config", body)
	req.Header.Set("Content-Type", MimeJSON)
	res := httptest.NewRecorder()
	logConfigHandler(res, req)

	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.Contains(t, string(data), "# root comment")

	var document struct {
		Log map[string]string `yaml:"log"`
	}
	require.NoError(t, yaml.Unmarshal(data, &document))
	require.Equal(t, "info", document.Log["level"])
	require.Equal(t, "debug", document.Log["onvif"])
	require.Equal(t, "trace", document.Log["rtsp"])
	require.NotContains(t, document.Log, "exec")
	require.Equal(t, "text", document.Log["format"])
	require.Equal(t, "file:go2rtc.log", document.Log["output"])
	require.Equal(t, "UNIXMS", document.Log["time"])
}

func TestLogConfigHandlerRejectsInvalidLevel(t *testing.T) {
	configPath := setupLogConfigTest(t, "log:\n  level: info\n")
	original, err := os.ReadFile(configPath)
	require.NoError(t, err)

	body := bytes.NewBufferString(`{"level":"verbose","modules":{"onvif":"debug"}}`)
	req := httptest.NewRequest(http.MethodPut, "/api/log/config", body)
	res := httptest.NewRecorder()
	logConfigHandler(res, req)

	require.Equal(t, http.StatusBadRequest, res.Code)
	actual, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.Equal(t, original, actual)
}

func TestLogConfigHandlerRejectsReservedModule(t *testing.T) {
	configPath := setupLogConfigTest(t, "log:\n  level: info\n")
	original, err := os.ReadFile(configPath)
	require.NoError(t, err)

	body := bytes.NewBufferString(`{"level":"info","modules":{"output":"debug"}}`)
	req := httptest.NewRequest(http.MethodPut, "/api/log/config", body)
	res := httptest.NewRecorder()
	logConfigHandler(res, req)

	require.Equal(t, http.StatusBadRequest, res.Code)
	actual, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.Equal(t, original, actual)
}

func TestLogConfigHandlerWithoutConfigFile(t *testing.T) {
	oldConfigPath := app.ConfigPath
	t.Cleanup(func() { app.ConfigPath = oldConfigPath })
	app.ConfigPath = ""

	req := httptest.NewRequest(http.MethodGet, "/api/log/config", nil)
	res := httptest.NewRecorder()
	logConfigHandler(res, req)

	require.Equal(t, http.StatusGone, res.Code)
}

func setupLogConfigTest(t *testing.T, content string) string {
	t.Helper()
	oldConfigPath := app.ConfigPath
	t.Cleanup(func() { app.ConfigPath = oldConfigPath })

	configPath := filepath.Join(t.TempDir(), "go2rtc.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(content), 0644))
	app.ConfigPath = configPath
	return configPath
}
