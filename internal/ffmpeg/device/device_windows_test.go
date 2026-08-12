//go:build windows

package device

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseMaxVideoMode(t *testing.T) {
	output := []byte(`[dshow] pixel_format=yuyv422 min s=640x360 fps=15 max s=640x360 fps=60
[dshow] pixel_format=nv12 min s=1920x1080 fps=15 max s=1920x1080 fps=25
[dshow] pixel_format=yuyv422 min s=1920x1080 fps=15 max s=1920x1080 fps=30`)

	size, framerate := parseMaxVideoMode(output)
	require.Equal(t, "1920x1080", size)
	require.Equal(t, "30", framerate)
}

func TestParseMaxVideoModeEmpty(t *testing.T) {
	size, framerate := parseMaxVideoMode([]byte("no supported modes"))
	require.Empty(t, size)
	require.Empty(t, framerate)
}
