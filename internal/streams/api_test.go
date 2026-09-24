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
	"github.com/AlexxIT/go2rtc/pkg/core"
	pkgrtsp "github.com/AlexxIT/go2rtc/pkg/rtsp"
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
	require.JSONEq(t, `{
        "streams_enabled": true,
        "onvif_enabled": true,
        "rtsp_enabled": true,
        "disabled_streams": ["camera1", "camera2"]
    }`, res.Body.String())
}

func TestAPIStreamsDOTIncludesTotalTrafficHeader(t *testing.T) {
	stream := &Stream{
		producers: []*Producer{{conn: &pkgrtsp.Conn{Connection: core.Connection{Recv: 1250}}}},
		consumers: []core.Consumer{&pkgrtsp.Conn{Connection: core.Connection{Send: 2250}}},
	}
	withStreamStateTestData(t, map[string]*Stream{"camera": stream}, map[string]bool{})

	req := httptest.NewRequest(http.MethodGet, "/api/streams.dot", nil)
	res := httptest.NewRecorder()
	apiStreamsDOT(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	require.Equal(t, "2250", res.Header().Get("X-Go2RTC-Total-Traffic-Bytes"))
}

func TestChangeGlobalStreamsStatePreservesIndividualDisabledState(t *testing.T) {
	HandleFunc("globaltest", func(string) (core.Producer, error) { return nil, nil })
	t.Cleanup(func() { delete(handlers, "globaltest") })

	configured := map[string][]string{
		"camera1": {"globaltest:camera1"},
		"camera2": {"globaltest:camera2"},
	}
	withStreamStateTestData(t, map[string]*Stream{
		"camera1": NewStream(configured["camera1"]),
	}, map[string]bool{"camera2": true})

	response, err := changeGlobalState("streams", false, configured, func(path []string, enabled bool) error {
		require.Equal(t, []string{"simulate", "streams_enabled"}, path)
		require.False(t, enabled)
		return nil
	})
	require.NoError(t, err)
	require.False(t, response.StreamsEnabled)
	require.Nil(t, Get("camera1"))
	require.Equal(t, []string{"camera2"}, DisabledNames())

	response, err = changeGlobalState("streams", true, configured, func([]string, bool) error { return nil })
	require.NoError(t, err)
	require.True(t, response.StreamsEnabled)
	require.NotNil(t, Get("camera1"))
	require.Nil(t, Get("camera2"))
	require.Equal(t, []string{"camera2"}, DisabledNames())
}

func TestChangeProtocolState(t *testing.T) {
	previous := true
	RegisterStateControl("onvif", func() bool { return previous }, func(enabled bool) { previous = enabled })
	t.Cleanup(func() {
		streamsMu.Lock()
		delete(streamStateControls, "onvif")
		streamsMu.Unlock()
	})

	response, err := changeGlobalState("onvif", false, nil, func(path []string, enabled bool) error {
		require.Equal(t, []string{"simulate", "onvif_enabled"}, path)
		require.False(t, enabled)
		return nil
	})
	require.NoError(t, err)
	require.False(t, previous)
	require.False(t, response.ONVIFEnabled)
}

func TestChangeGlobalStreamsStateValidatesBeforePersistence(t *testing.T) {
	withStreamStateTestData(t, map[string]*Stream{}, map[string]bool{})
	require.NoError(t, SetAllEnabled(false, nil))
	persisted := false

	_, err := changeGlobalState("streams", true, map[string][]string{
		"camera1": {"unsupported:camera1"},
	}, func([]string, bool) error {
		persisted = true
		return nil
	})

	require.EqualError(t, err, "streams: source not supported")
	require.False(t, persisted)
	require.False(t, Enabled())
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
	previousEnabled := streamsEnabled
	streams = testStreams
	disabledStreams = testDisabled
	streamOrder = nil
	streamsEnabled = true
	streamsMu.Unlock()

	t.Cleanup(func() {
		streamsMu.Lock()
		for _, stream := range streams {
			stream.Close()
		}
		streams = previousStreams
		disabledStreams = previousDisabled
		streamOrder = previousOrder
		streamsEnabled = previousEnabled
		streamsMu.Unlock()
	})
}
