package onvif

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestApplyONVIFStreamQuality(t *testing.T) {
	uri := applyONVIFStreamQuality("rtsp://192.0.2.10:9554/camera1", onvifStreamQuality{Height: 720})
	require.Equal(t, "rtsp://192.0.2.10:9554/camera1?onvif_height=720", uri)

	uri = applyONVIFStreamQuality("rtsp://192.0.2.10:9554/camera1", onvifStreamQuality{Width: 1280, Height: 720})
	require.Contains(t, uri, "onvif_width=1280")
	require.Contains(t, uri, "onvif_height=720")
}

func TestApplyRTSPAuth(t *testing.T) {
	uri := applyRTSPAuth("rtsp://192.0.2.10:9554/camera1", rtspAuthConfig{Username: "admin", Password: "secret"})
	require.Equal(t, "rtsp://admin:secret@192.0.2.10:9554/camera1", uri)

	uri = applyRTSPAuth("rtsp://192.0.2.10:9554/camera1", rtspAuthConfig{})
	require.Equal(t, "rtsp://192.0.2.10:9554/camera1", uri)
}

func TestONVIFProfileForQuality(t *testing.T) {
	original := onvifProfileForQuality("camera1", onvifStreamQuality{})
	require.Equal(t, "camera1", original.Token)
	require.Equal(t, "camera1", original.SourceToken)

	profile := onvifProfileForQuality("camera1", onvifStreamQuality{Height: 720})
	require.Equal(t, "camera1 720p", profile.Name)
	require.Equal(t, "camera1__onvif_720p", profile.Token)
	require.Equal(t, "camera1", profile.SourceToken)
	require.Equal(t, 720, profile.Height)
}
