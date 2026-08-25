package streams

import (
	"errors"
	"net/http"
	"net/http/httptest"
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

func TestAPIStreamStateGet(t *testing.T) {
	withStreamStateTestData(t, map[string]*Stream{}, map[string]bool{"camera2": true, "camera1": true})

	req := httptest.NewRequest(http.MethodGet, "/api/streams/state", nil)
	res := httptest.NewRecorder()
	apiStreamState(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	require.JSONEq(t, `{"disabled_streams":["camera1","camera2"]}`, res.Body.String())
}

func TestChangeStreamStateWithReadOnlyConfig(t *testing.T) {
	const name = "linux-camera"
	withStreamStateTestData(t, map[string]*Stream{name: NewStream("ffmpeg:test")}, map[string]bool{})

	response, err := changeStreamState(name, []string{"ffmpeg:test"}, false, func([]string) error {
		return errors.New("open /config/go2rtc.yaml: read-only file system")
	})

	require.NoError(t, err)
	require.False(t, response.Enabled)
	require.False(t, response.Persisted)
	require.Contains(t, response.Warning, "until restart")
	require.Equal(t, []string{name}, response.DisabledStreams)
	require.True(t, IsDisabled(name))
	require.Nil(t, Get(name))
}

func TestChangeStreamStateRejectsOtherConfigErrors(t *testing.T) {
	const name = "invalid-config-camera"
	stream := NewStream("ffmpeg:test")
	withStreamStateTestData(t, map[string]*Stream{name: stream}, map[string]bool{})

	_, err := changeStreamState(name, []string{"ffmpeg:test"}, false, func([]string) error {
		return errors.New("yaml: invalid document")
	})

	require.EqualError(t, err, "yaml: invalid document")
	require.False(t, IsDisabled(name))
	require.Same(t, stream, Get(name))
}

func withStreamStateTestData(t *testing.T, testStreams map[string]*Stream, testDisabled map[string]bool) {
	t.Helper()
	streamsMu.Lock()
	previousStreams := streams
	previousDisabled := disabledStreams
	previousOrder := streamOrder
	streams = testStreams
	disabledStreams = testDisabled
	streamOrder = nil
	streamsMu.Unlock()

	t.Cleanup(func() {
		streamsMu.Lock()
		for _, stream := range streams {
			stream.Close()
		}
		streams = previousStreams
		disabledStreams = previousDisabled
		streamOrder = previousOrder
		streamsMu.Unlock()
	})
}
