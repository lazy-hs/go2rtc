package rtsp

import (
	"bytes"
	"errors"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestONVIFPTZZMQUpdate(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	endpoint, err := allocateONVIFPTZEndpoint()
	require.NoError(t, err)
	escapedEndpoint := strings.ReplaceAll(endpoint, ":", "\\\\:")

	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
		"-re", "-f", "lavfi", "-i", "testsrc2=size=320x240:rate=10", "-t", "10",
		"-vf", "scale@ptz=320:-2:eval=frame,crop@ptz=320:240:0:0,zmq=bind_address="+escapedEndpoint,
		"-f", "null", "-")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	})

	item := &onvifPTZStream{endpoint: endpoint}
	t.Cleanup(item.resetSocket)
	var applyErr error
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		applyErr = item.apply(onvifPTZState{Pan: 0.5, Tilt: -0.5, Zoom: 0.5, MaxZoom: 4})
		if applyErr == nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.NoError(t, applyErr, stderr.String())
}

func TestONVIFPTZInactiveStreamUsesLatestPosition(t *testing.T) {
	onvifPTZRegistry.Lock()
	previousEnabled := onvifPTZRegistry.enabled
	previousStates := onvifPTZRegistry.states
	previousStreams := onvifPTZRegistry.streams
	onvifPTZRegistry.enabled = true
	onvifPTZRegistry.states = map[string]onvifPTZState{}
	onvifPTZRegistry.streams = map[onvifPTZStreamKey]*onvifPTZStream{}
	onvifPTZRegistry.Unlock()
	t.Cleanup(func() {
		onvifPTZRegistry.Lock()
		onvifPTZRegistry.enabled = previousEnabled
		onvifPTZRegistry.states = previousStates
		onvifPTZRegistry.streams = previousStreams
		onvifPTZRegistry.Unlock()
	})

	ConfigureONVIFPTZ("main", 4, 0, 0, 0)
	stream := onvifPTZStreamFor("main", streamQuality{})
	require.NotNil(t, stream)
	require.Contains(t, stream.Sources()[0], "ptz_zoom=0.000000")
	require.Contains(t, stream.Sources()[0], "ptz_fps=30")
	require.Contains(t, stream.Sources()[0], "width=1920")
	require.Contains(t, stream.Sources()[0], "height=1080")

	UpdateONVIFPTZ("main", 4, 1, 0.5, 0.75)
	require.Contains(t, stream.Sources()[0], "ptz_pan=1.000000")
	require.Contains(t, stream.Sources()[0], "ptz_zoom=0.750000")
}

func TestONVIFPTZGlobalSwitchClosesDerivedStreams(t *testing.T) {
	onvifPTZRegistry.Lock()
	previousEnabled := onvifPTZRegistry.enabled
	previousStates := onvifPTZRegistry.states
	previousStreams := onvifPTZRegistry.streams
	onvifPTZRegistry.enabled = true
	onvifPTZRegistry.states = map[string]onvifPTZState{}
	onvifPTZRegistry.streams = map[onvifPTZStreamKey]*onvifPTZStream{}
	onvifPTZRegistry.Unlock()
	t.Cleanup(func() {
		SetONVIFPTZEnabled(false)
		onvifPTZRegistry.Lock()
		onvifPTZRegistry.enabled = previousEnabled
		onvifPTZRegistry.states = previousStates
		onvifPTZRegistry.streams = previousStreams
		onvifPTZRegistry.Unlock()
	})

	ConfigureONVIFPTZ("main", 4, 0, 0, 0)
	stream := onvifPTZStreamFor("main", streamQuality{})
	require.NotNil(t, stream)

	SetONVIFPTZEnabled(false)
	require.Nil(t, onvifPTZStreamFor("main", streamQuality{}))
	onvifPTZRegistry.Lock()
	require.Empty(t, onvifPTZRegistry.streams)
	onvifPTZRegistry.Unlock()

	SetONVIFPTZEnabled(true)
	require.NotNil(t, onvifPTZStreamFor("main", streamQuality{}))
}

func TestONVIFPTZUpdateRetriesUntilFilterIsReady(t *testing.T) {
	var attempts atomic.Int32
	done := make(chan struct{})
	item := &onvifPTZStream{
		updates: make(chan onvifPTZState, 1),
		applyFn: func(onvifPTZState) error {
			if attempts.Add(1) < 3 {
				return errors.New("filter is starting")
			}
			close(done)
			return nil
		},
	}
	go item.run()
	item.queue(onvifPTZState{Pan: 0.5})

	select {
	case <-done:
		require.Equal(t, int32(3), attempts.Load())
	case <-time.After(time.Second):
		t.Fatal("PTZ filter update was not retried")
	}
}
