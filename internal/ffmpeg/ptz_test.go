package ffmpeg

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPTZCropFilter(t *testing.T) {
	require.Equal(t, "scale@ptz=floor(iw*2.500000/2)*2:-2:eval=frame", ptzZoomFilter(0.5, 4))
	require.Equal(t, "crop@ptz=1920:1080:(iw-ow)*1.000000:(ih-oh)*1.000000", ptzCropFilter(1, -1, 1920, 1080))
}

func TestEscapeZMQFilterEndpoint(t *testing.T) {
	require.Equal(t, `tcp\\://127.0.0.1\\:5555`, escapeZMQFilterEndpoint("tcp://127.0.0.1:5555"))
}

func TestPTZFilterLimitsOutputFrameRate(t *testing.T) {
	args := parseArgs("input.mp4#video=h264#width=1920#height=1080#ptz_endpoint=tcp://127.0.0.1:5555#ptz_fps=30")
	require.Contains(t, args.String(), `scale=1920:1080:eval=frame`)
	require.Contains(t, args.String(), `fps=30,zmq=bind_address=tcp\\://127.0.0.1\\:5555`)
}
