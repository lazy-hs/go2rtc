package api

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/stretchr/testify/require"
)

func TestSimulateHandlerUsesRequestHostAndBasePath(t *testing.T) {
	oldBasePath := basePath
	oldConfigPath := app.ConfigPath
	oldUploadDir := simulateUploadDir
	oldRTSP, hadRTSP := app.Info["rtsp"]
	t.Cleanup(func() {
		basePath = oldBasePath
		app.ConfigPath = oldConfigPath
		simulateUploadDir = oldUploadDir
		if hadRTSP {
			app.Info["rtsp"] = oldRTSP
		} else {
			delete(app.Info, "rtsp")
		}
	})

	basePath = "/rtc"
	app.ConfigPath = ""
	delete(app.Info, "rtsp")
	simulateUploadDir = filepath.Join(t.TempDir(), "static")
	app.Info["host"] = "stale.example:1984"

	req := httptest.NewRequest("GET", "http://ignored/rtc/api/simulate", nil)
	req.Host = "192.0.2.20:1984"
	res := httptest.NewRecorder()
	simulateHandler(res, req)

	require.Equal(t, 200, res.Code)
	var info simulateInfo
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &info))
	require.Equal(t, "192.0.2.20:1984", info.Host)
	require.Equal(t, "/rtc", info.BasePath)
	require.Equal(t, "/rtc/api/streams", info.StreamsAPI)
	require.Equal(t, "/rtc/api/log", info.LogAPI)
	require.Equal(t, "/rtc/api/simulate/metrics", info.MetricsAPI)
	require.Equal(t, "/rtc/api/ffmpeg/devices", info.DevicesAPI)
	require.Equal(t, "/rtc/api/simulate/onvif", info.ONVIFConfigAPI)
	require.Equal(t, "/rtc/api/streams/state", info.StreamStateAPI)
	require.Equal(t, "/rtc/api/simulate/events", info.EventsAPI)
	require.Equal(t, "/rtc/api/simulate/files", info.FilesAPI)
	require.Equal(t, "/rtc/api/simulate/upload", info.UploadAPI)
	require.Equal(t, "/rtc/onvif/device_service", info.ONVIFPath)
	require.Equal(t, "8554", info.RTSPPort)
}

func TestConfiguredDisabledStreamsFromFile(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "go2rtc.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`simulate:
  disabled_streams:
    - camera2
    - rtsp-main
`), 0644))

	require.Equal(t, []string{"camera2", "rtsp-main"}, configuredDisabledStreamsFromFile(configPath))
}

func TestConfiguredONVIFQualitiesFromFile(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "go2rtc.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`simulate:
  onvif_qualities:
    camera1:
      - {}
      - height: 720
    camera2:
      - width: 1280
        height: 720
    empty:
      []
`), 0644))

	qualities := configuredONVIFQualitiesFromFile(configPath)
	require.Equal(t, []simulateONVIFStreamQuality{{}, {Height: 720}}, qualities["camera1"])
	require.Equal(t, []simulateONVIFStreamQuality{{Width: 1280, Height: 720}}, qualities["camera2"])
	require.NotContains(t, qualities, "empty")
}

func TestConfiguredONVIFQualitiesFromFileLegacy(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "go2rtc.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`simulate:
  onvif_quality:
    camera1:
      height: 720
    empty:
      height: 0
`), 0644))

	qualities := configuredONVIFQualitiesFromFile(configPath)
	require.Equal(t, []simulateONVIFStreamQuality{{Height: 720}}, qualities["camera1"])
	require.NotContains(t, qualities, "empty")
}

func TestSimulateHandlerUsesConfiguredRTSPPort(t *testing.T) {
	oldConfigPath := app.ConfigPath
	oldRTSP, hadRTSP := app.Info["rtsp"]
	t.Cleanup(func() {
		app.ConfigPath = oldConfigPath
		if hadRTSP {
			app.Info["rtsp"] = oldRTSP
		} else {
			delete(app.Info, "rtsp")
		}
	})

	configPath := filepath.Join(t.TempDir(), "go2rtc.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("rtsp:\n  listen: \":9554\"\n"), 0644))
	app.ConfigPath = configPath
	delete(app.Info, "rtsp")

	req := httptest.NewRequest("GET", "http://ignored/api/simulate", nil)
	res := httptest.NewRecorder()
	simulateHandler(res, req)

	require.Equal(t, 200, res.Code)
	var info simulateInfo
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &info))
	require.Equal(t, "9554", info.RTSPPort)
}

func TestSimulateRTSPPortUsesEffectiveRuntimeConfig(t *testing.T) {
	oldRTSP, hadRTSP := app.Info["rtsp"]
	t.Cleanup(func() {
		if hadRTSP {
			app.Info["rtsp"] = oldRTSP
		} else {
			delete(app.Info, "rtsp")
		}
	})

	app.Info["rtsp"] = map[string]any{"listen": "[::]:10554"}
	require.Equal(t, "10554", simulateRTSPPort(""))
}

func TestConfiguredStreamsFromFile(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "go2rtc.yaml")
	config := []byte(`streams:
  camera1: ffmpeg:D:/media/1.mp4#video=h264#input=file
  camera2: ffmpeg:D:/media/2.mp4#video=h264#input=file
  camera3: ffmpeg:D:/media/3.mp4#video=h264#input=file
  camera4: ffmpeg:D:/media/4.mp4#video=h264#input=file
  camera5: ffmpeg:D:/media/5.mp4#video=h264#input=file
  camera6: ffmpeg:D:/media/6.mp4#video=h264#input=file
  camera7:
    - ffmpeg:D:/media/7.mp4#video=h265#input=file
    - rtsp://example.test/live
  unsupported:
    url: ffmpeg:D:/media/object.mp4#input=file
`)
	require.NoError(t, os.WriteFile(configPath, config, 0644))

	streams, order := configuredStreamsFromFile(configPath)
	require.Len(t, streams, 7)
	require.Equal(t, []string{"camera1", "camera2", "camera3", "camera4", "camera5", "camera6", "camera7"}, order)
	require.Equal(t, []string{"ffmpeg:D:/media/1.mp4#video=h264#input=file"}, streams["camera1"])
	require.Equal(t, []string{
		"ffmpeg:D:/media/7.mp4#video=h265#input=file",
		"rtsp://example.test/live",
	}, streams["camera7"])
	require.NotContains(t, streams, "unsupported")
}
