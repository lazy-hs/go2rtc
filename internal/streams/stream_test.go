package streams

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/stretchr/testify/require"
)

func TestGetAllNamesUsesStableChannelOrder(t *testing.T) {
	streamsMu.Lock()
	previousStreams := streams
	previousOrder := streamOrder
	streams = map[string]*Stream{
		"camera3": {},
		"camera1": {},
		"camera2": {},
		"extra-z": {},
		"extra-a": {},
	}
	streamOrder = []string{"camera2", "camera1", "camera3"}
	streamsMu.Unlock()

	t.Cleanup(func() {
		streamsMu.Lock()
		streams = previousStreams
		streamOrder = previousOrder
		streamsMu.Unlock()
	})

	require.Equal(t, []string{"camera2", "camera1", "camera3", "extra-a", "extra-z"}, GetAllNames())
	require.Equal(t, []string{"camera2", "camera1", "camera3", "extra-a", "extra-z"}, GetAllNames())

	Delete("camera1")
	require.Equal(t, []string{"camera2", "camera3", "extra-a", "extra-z"}, GetAllNames())
}

func TestStreamOrderFromConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "go2rtc.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`streams:
  main: rtsp://example/main
  camera10: rtsp://example/10
  camera2: rtsp://example/2
`), 0644))

	require.Equal(t, []string{"main", "camera10", "camera2"}, streamOrderFromConfig(path))
}

func TestRecursion(t *testing.T) {
	HandleFunc("rtsp", func(url string) (core.Producer, error) { return nil, nil })

	// create stream with some source
	stream1, err := New("from_yaml", "rtsp://example.com/live")
	require.NoError(t, err)
	require.Len(t, streams, 1)

	// ask another unnamed stream that links go2rtc
	query, err := url.ParseQuery("src=rtsp://localhost:8554/from_yaml?video")
	require.NoError(t, err)
	stream2, err := GetOrPatch(query)
	require.NoError(t, err)

	// check stream is same
	require.Equal(t, stream1, stream2)
	// check stream urls is same
	require.Equal(t, stream1.producers[0].url, stream2.producers[0].url)
	require.Len(t, streams, 2)
}

func TestTempate(t *testing.T) {
	HandleFunc("rtsp", func(url string) (core.Producer, error) { return nil, nil }) // bypass HasProducer
	HandleFunc("ffmpeg", func(url string) (core.Producer, error) { return nil, nil })

	// config from yaml
	stream1, err := New("camera.from_hass", "ffmpeg:{input}#video=copy")
	require.NoError(t, err)
	// request from hass
	stream2, err := Patch("camera.from_hass", "rtsp://example.com")
	require.NoError(t, err)

	require.Equal(t, stream1, stream2)
	require.Equal(t, "ffmpeg:rtsp://example.com#video=copy", stream1.producers[0].url)
}
