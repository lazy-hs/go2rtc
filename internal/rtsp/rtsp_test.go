package rtsp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/stretchr/testify/require"
)

func TestResolveRTSPQualityAlias(t *testing.T) {
	oldConfigPath := app.ConfigPath
	t.Cleanup(func() {
		app.ConfigPath = oldConfigPath
	})

	configPath := filepath.Join(t.TempDir(), "go2rtc.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`simulate:
  onvif_qualities:
    main:
      - {}
      - height: 720
      - height: 2048
      - width: 1920
        height: 1080
`), 0644))
	app.ConfigPath = configPath

	name, quality, ok := resolveRTSPQualityAlias("main_720p", []string{"main"})
	require.True(t, ok)
	require.Equal(t, "main", name)
	require.Equal(t, streamQuality{Width: 1280, Height: 720}, quality)

	name, quality, ok = resolveRTSPQualityAlias("main_2048p", []string{"main"})
	require.True(t, ok)
	require.Equal(t, "main", name)
	require.Equal(t, streamQuality{Width: 3642, Height: 2048}, quality)

	name, quality, ok = resolveRTSPQualityAlias("main_1920x1080", []string{"main"})
	require.True(t, ok)
	require.Equal(t, "main", name)
	require.Equal(t, streamQuality{Width: 1920, Height: 1080}, quality)

	_, _, ok = resolveRTSPQualityAlias("main_480p", []string{"main"})
	require.False(t, ok)
}

func TestNormalizeRTSPQualityUsesEvenWidth(t *testing.T) {
	require.Equal(t, streamQuality{Width: 3642, Height: 2048}, normalizeRTSPQuality(streamQuality{Height: 2048}))
	require.Equal(t, streamQuality{Width: 1280, Height: 720}, normalizeRTSPQuality(streamQuality{Height: 720}))
}

func TestLocalURLUsesConfiguredAuth(t *testing.T) {
	oldConfigPath := app.ConfigPath
	oldPort := Port
	t.Cleanup(func() {
		app.ConfigPath = oldConfigPath
		Port = oldPort
	})

	configPath := filepath.Join(t.TempDir(), "go2rtc.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`rtsp:
  username: lazy
  password: Aa123456
`), 0644))
	app.ConfigPath = configPath
	Port = "9554"

	require.Equal(t, "rtsp://lazy:Aa123456@127.0.0.1:9554/main", LocalURL("main"))
	require.Equal(t, "rtsp://lazy:Aa123456@127.0.0.1:9554/internal/path", LocalURL("/internal/path"))
}
