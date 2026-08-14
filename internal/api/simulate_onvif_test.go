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

func TestSimulateONVIFConfigHandlerGet(t *testing.T) {
	configPath := setupSimulateONVIFConfigTest(t, `onvif:
  name: Warehouse Camera
  manufacturer: HuangSheng
  model: Virtual IPC-9000
  firmware: 2.1.0
  serial: CAM-001
  hardware: Virtual IPC
api:
  listen: ":2984"
rtsp:
  listen: ":9554"
`)

	req := httptest.NewRequest(http.MethodGet, "/api/simulate/onvif", nil)
	res := httptest.NewRecorder()
	simulateONVIFConfigHandler(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	var response simulateONVIFConfigResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &response))
	require.Equal(t, "Warehouse Camera", response.Config.Name)
	require.Equal(t, "HuangSheng", response.Config.Manufacturer)
	require.Equal(t, "Virtual IPC-9000", response.Effective.Model)
	require.Equal(t, 2984, response.Config.ServicePort)
	require.Equal(t, 9554, response.Config.RTSPPort)
	require.FileExists(t, configPath)
}

func TestSimulateONVIFConfigHandlerPutPreservesOtherSettings(t *testing.T) {
	configPath := setupSimulateONVIFConfigTest(t, `# config comment
onvif:
  name: Old Camera
  manufacturer: Old Vendor
  custom: keep-me
api:
  listen: ":2984"
rtsp:
  listen: ":9554"
`)

	body := bytes.NewBufferString(`{"name":"仓库模拟摄像机","manufacturer":"HuangSheng","model":"Virtual IPC-9000","firmware":"2.1.0","serial":"CAM-001","hardware":"Virtual IPC","service_port":3984,"rtsp_port":10554}`)
	req := httptest.NewRequest(http.MethodPut, "/api/simulate/onvif", body)
	res := httptest.NewRecorder()
	simulateONVIFConfigHandler(res, req)

	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.Contains(t, string(data), "# config comment")

	var document struct {
		ONVIF map[string]string `yaml:"onvif"`
		API   map[string]string `yaml:"api"`
		RTSP  map[string]string `yaml:"rtsp"`
	}
	require.NoError(t, yaml.Unmarshal(data, &document))
	require.Equal(t, "仓库模拟摄像机", document.ONVIF["name"])
	require.Equal(t, "HuangSheng", document.ONVIF["manufacturer"])
	require.Equal(t, "Virtual IPC-9000", document.ONVIF["model"])
	require.Equal(t, "keep-me", document.ONVIF["custom"])
	require.Equal(t, ":3984", document.API["listen"])
	require.Equal(t, ":10554", document.RTSP["listen"])
}

func TestSimulateONVIFConfigHandlerRemovesEmptyOverride(t *testing.T) {
	configPath := setupSimulateONVIFConfigTest(t, "onvif:\n  name: Camera\n  serial: CAM-001\n")
	body := bytes.NewBufferString(`{"name":"Camera","manufacturer":"","model":"","firmware":"","serial":"","hardware":""}`)
	req := httptest.NewRequest(http.MethodPut, "/api/simulate/onvif", body)
	res := httptest.NewRecorder()
	simulateONVIFConfigHandler(res, req)

	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	var document struct {
		ONVIF map[string]string `yaml:"onvif"`
	}
	require.NoError(t, yaml.Unmarshal(data, &document))
	require.NotContains(t, document.ONVIF, "serial")
}

func TestSimulateONVIFConfigHandlerWithoutConfig(t *testing.T) {
	oldConfigPath := app.ConfigPath
	t.Cleanup(func() { app.ConfigPath = oldConfigPath })
	app.ConfigPath = ""

	req := httptest.NewRequest(http.MethodGet, "/api/simulate/onvif", nil)
	res := httptest.NewRecorder()
	simulateONVIFConfigHandler(res, req)
	require.Equal(t, http.StatusGone, res.Code)
}

func setupSimulateONVIFConfigTest(t *testing.T, content string) string {
	t.Helper()
	oldConfigPath := app.ConfigPath
	t.Cleanup(func() { app.ConfigPath = oldConfigPath })
	configPath := filepath.Join(t.TempDir(), "go2rtc.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(content), 0644))
	app.ConfigPath = configPath
	return configPath
}
