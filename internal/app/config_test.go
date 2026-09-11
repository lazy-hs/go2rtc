package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPlatformStreamConfig(t *testing.T) {
	require.Equal(t, "go2rtc_linux.yaml", platformStreamConfig("linux"))
	require.Equal(t, "go2rtc_windows.yaml", platformStreamConfig("windows"))
	require.Equal(t, "go2rtc_mac.yaml", platformStreamConfig("darwin"))
	require.Empty(t, platformStreamConfig("freebsd"))
}

func TestConfigContainsStreamSettings(t *testing.T) {
	require.True(t, configContainsStreamSettings([]byte("streams:\n  camera: rtsp://example\n")))
	require.True(t, configContainsStreamSettings([]byte("simulate:\n  streams_enabled: true\n")))
	require.False(t, configContainsStreamSettings([]byte("api:\n  listen: ':1984'\n")))
	require.False(t, configContainsStreamSettings([]byte("invalid: [")))
}

func TestPatchConfigRoutesStreamSettingsToStreamConfig(t *testing.T) {
	projectPath := filepath.Join(t.TempDir(), "go2rtc.yaml")
	streamPath := filepath.Join(t.TempDir(), "go2rtc_linux.yaml")
	require.NoError(t, os.WriteFile(projectPath, []byte("log:\n  level: info\n"), 0644))
	require.NoError(t, os.WriteFile(streamPath, []byte("streams:\n  camera: old\n"), 0644))

	previousConfigPath := ConfigPath
	previousStreamConfigPath := StreamConfigPath
	t.Cleanup(func() {
		ConfigPath = previousConfigPath
		StreamConfigPath = previousStreamConfigPath
	})
	ConfigPath = projectPath
	StreamConfigPath = streamPath

	require.NoError(t, PatchConfig([]string{"streams", "camera"}, "new"))
	require.NoError(t, PatchConfig([]string{"log", "level"}, "debug"))

	projectData, err := os.ReadFile(projectPath)
	require.NoError(t, err)
	streamData, err := os.ReadFile(streamPath)
	require.NoError(t, err)
	require.Contains(t, string(projectData), "level: debug")
	require.NotContains(t, string(projectData), "streams:")
	require.Contains(t, string(streamData), "camera: new")
}
