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
	t.Cleanup(func() {
		basePath = oldBasePath
		app.ConfigPath = oldConfigPath
		simulateUploadDir = oldUploadDir
	})

	basePath = "/rtc"
	app.ConfigPath = ""
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
	require.Equal(t, "/rtc/api/simulate/files", info.FilesAPI)
	require.Equal(t, "/rtc/api/simulate/upload", info.UploadAPI)
	require.Equal(t, "/rtc/onvif/device_service", info.ONVIFPath)
	require.Equal(t, "8554", info.RTSPPort)
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

	streams := configuredStreamsFromFile(configPath)
	require.Len(t, streams, 7)
	require.Equal(t, []string{"ffmpeg:D:/media/1.mp4#video=h264#input=file"}, streams["camera1"])
	require.Equal(t, []string{
		"ffmpeg:D:/media/7.mp4#video=h265#input=file",
		"rtsp://example.test/live",
	}, streams["camera7"])
	require.NotContains(t, streams, "unsupported")
}
