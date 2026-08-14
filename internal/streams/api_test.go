package streams

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/stretchr/testify/require"
)

func TestPatchONVIFStreamQualitiesCreatesMissingParentPath(t *testing.T) {
	oldConfigPath := app.ConfigPath
	t.Cleanup(func() {
		app.ConfigPath = oldConfigPath
	})

	configPath := filepath.Join(t.TempDir(), "go2rtc.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`simulate:
  disabled_streams: []
  onvif_quality:
    main:
      height: 720
`), 0644))
	app.ConfigPath = configPath

	query := url.Values{}
	query.Add("onvif_quality", "original")
	query.Add("onvif_quality", "1080")
	require.NoError(t, patchONVIFStreamQualities("main", query))

	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.Contains(t, string(data), "onvif_qualities:")
	require.Contains(t, string(data), "height: 1080")
	require.NotContains(t, string(data), "height: 720")
}
