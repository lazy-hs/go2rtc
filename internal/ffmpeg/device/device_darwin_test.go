//go:build darwin || ios

package device

import (
	"net/url"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQueryToInputUsesAvfoundationDeviceIndex(t *testing.T) {
	oldVideos, oldAudios, oldVideoModes := videos, audios, videoModes
	t.Cleanup(func() {
		videos, audios, videoModes = oldVideos, oldAudios, oldVideoModes
	})

	videos = []string{"0"}
	audios = []string{"1"}
	videoModes = map[string]avfoundationMode{
		"0": {Size: "1280x720", Framerate: "30"},
	}

	input := queryToInput(url.Values{
		"video": {"0"},
		"audio": {"1"},
	})

	require.Equal(t, `-f avfoundation -framerate 30 -video_size 1280x720 -i "0:1"`, input)
}

func TestQueryToInputKeepsExplicitFramerate(t *testing.T) {
	oldVideos, oldVideoModes := videos, videoModes
	t.Cleanup(func() {
		videos, videoModes = oldVideos, oldVideoModes
	})

	videos = []string{"0"}
	videoModes = map[string]avfoundationMode{
		"0": {Size: "1280x720", Framerate: "30"},
	}

	input := queryToInput(url.Values{
		"video":     {"0"},
		"framerate": {"25"},
	})

	require.Equal(t, `-f avfoundation -video_size 1280x720 -framerate 25 -i "0:"`, input)
}

func TestInitDevicesUsesIndicesAndStripsMetadata(t *testing.T) {
	script := t.TempDir() + "/ffmpeg"
	err := os.WriteFile(script, []byte(`#!/bin/sh
printf '%s\n' \
  '[AVFoundation indev @ 0x1] AVFoundation video devices:' \
  '[AVFoundation indev @ 0x1] [0] FaceTime高清相机（内建）  [uid:camera] [serial:0000]' \
  '[AVFoundation indev @ 0x1] AVFoundation audio devices:' \
  '[AVFoundation indev @ 0x1] [1] 外置麦克风  [uid:microphone]' \
  '[in#0 @ 0x1]   640x480@[30.000030 30.000030]fps' \
  '[in#0 @ 0x1]   1280x720@[30.000030 30.000030]fps' >&2
`), 0755)
	require.NoError(t, err)

	oldBin, oldVideos, oldAudios, oldStreams := Bin, videos, audios, streams
	t.Cleanup(func() {
		Bin, videos, audios, streams = oldBin, oldVideos, oldAudios, oldStreams
	})

	Bin = script
	videos, audios, streams = nil, nil, nil
	initDevices()

	require.Equal(t, []string{"0"}, videos)
	require.Equal(t, []string{"1"}, audios)
	require.Len(t, streams, 2)
	require.Equal(t, "FaceTime高清相机（内建）", streams[0].Name)
	require.Equal(t, "ffmpeg:device?video=0&video_size=1280x720&framerate=30", streams[0].URL)
	require.Contains(t, streams[0].Info, "uid=camera")
	require.Contains(t, streams[0].Info, "serial=0000")
	require.Contains(t, streams[0].Info, "selected=1280x720@30")
	require.Equal(t, "外置麦克风", streams[1].Name)
	require.Equal(t, "ffmpeg:device?audio=1", streams[1].URL)
}

func TestParseAndSelectAVFoundationModes(t *testing.T) {
	modes := parseAVFoundationModes([]byte(`
[in#0 @ 0x1]   640x480@[30.000030 30.000030]fps
[in#0 @ 0x1]   1280x720@[15.000015 15.000015]fps
[in#0 @ 0x1]   1280x720@[30.000030 30.000030]fps
`))

	require.Len(t, modes, 3)
	selected := selectAVFoundationMode(modes)
	require.Equal(t, "1280x720", selected.Size)
	require.Equal(t, "30", selected.Framerate)
	require.Equal(t, "selected=1280x720@30, supported=1280x720@15-30,640x480@30", formatAVFoundationModes(modes, selected))
}
