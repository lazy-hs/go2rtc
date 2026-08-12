//go:build windows

package device

import (
	"net/url"
	"os/exec"
	"regexp"
	"strconv"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/pkg/core"
)

func queryToInput(query url.Values) string {
	video := query.Get("video")
	audio := query.Get("audio")

	if video == "" && audio == "" {
		return ""
	}

	// https://ffmpeg.org/ffmpeg-devices.html#dshow
	input := "-f dshow"

	if video != "" {
		video = indexToItem(videos, video)

		for key, value := range query {
			switch key {
			case "resolution":
				input += " -video_size " + value[0]
			case "video_size", "framerate", "pixel_format":
				input += " -" + key + " " + value[0]
			}
		}
	}

	if audio != "" {
		audio = indexToItem(audios, audio)

		for key, value := range query {
			switch key {
			case "sample_rate", "sample_size", "channels", "audio_buffer_size":
				input += " -" + key + " " + value[0]
			}
		}
	}

	if video != "" {
		input += ` -i "video=` + video

		if audio != "" {
			input += `:audio=` + audio
		}

		input += `"`
	} else {
		input += ` -i "audio=` + audio + `"`
	}

	return input
}

func initDevices() {
	cmd := exec.Command(
		Bin, "-hide_banner", "-list_devices", "true", "-f", "dshow", "-i", "",
	)
	b, _ := cmd.CombinedOutput()

	re := regexp.MustCompile(`"([^"]+)" \((video|audio)\)`)
	for _, m := range re.FindAllStringSubmatch(string(b), -1) {
		name := m[1]
		kind := m[2]

		stream := &api.Source{
			Name: name, URL: "ffmpeg:device?" + kind + "=" + url.QueryEscape(name),
		}

		switch kind {
		case core.KindVideo:
			videos = append(videos, name)
			if size, framerate := probeMaxVideoMode(name); size != "" {
				stream.URL += "&video_size=" + size
				if framerate != "" {
					stream.URL += "&framerate=" + framerate
				}
			}
			stream.URL += "#video=h264#hardware"
		case core.KindAudio:
			audios = append(audios, name)
			stream.URL += "&channels=1&sample_rate=16000&audio_buffer_size=10"
		}

		streams = append(streams, stream)
	}
}

func probeMaxVideoMode(name string) (size, framerate string) {
	cmd := exec.Command(
		Bin, "-hide_banner", "-f", "dshow", "-list_options", "true", "-i", "video="+name,
	)
	b, _ := cmd.CombinedOutput()
	return parseMaxVideoMode(b)
}

func parseMaxVideoMode(b []byte) (size, framerate string) {
	re := regexp.MustCompile(`\bs=(\d+)x(\d+)\s+fps=([0-9]+(?:\.[0-9]+)?)`)
	maxArea := 0
	maxFPS := 0.0

	for _, match := range re.FindAllSubmatch(b, -1) {
		width, _ := strconv.Atoi(string(match[1]))
		height, _ := strconv.Atoi(string(match[2]))
		fps, _ := strconv.ParseFloat(string(match[3]), 64)
		area := width * height
		if area < maxArea || area == maxArea && fps <= maxFPS {
			continue
		}

		maxArea = area
		maxFPS = fps
		size = string(match[1]) + "x" + string(match[2])
		framerate = string(match[3])
	}

	return
}
