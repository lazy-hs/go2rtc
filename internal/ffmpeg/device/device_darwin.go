//go:build darwin || ios

package device

import (
	"context"
	"math"
	"net/url"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/pkg/core"
)

func queryToInput(query url.Values) string {
	video := query.Get("video")
	audio := query.Get("audio")

	if video == "" && audio == "" {
		return ""
	}

	// https://ffmpeg.org/ffmpeg-devices.html#avfoundation
	input := "-f avfoundation"

	if video != "" {
		video = indexToItem(videos, video)

		if query.Get("framerate") == "" {
			if mode := videoModes[video]; mode.Framerate != "" {
				input += " -framerate " + mode.Framerate
			}
		}
		if query.Get("resolution") == "" && query.Get("video_size") == "" {
			if mode := videoModes[video]; mode.Size != "" {
				input += " -video_size " + mode.Size
			}
		}

		for key, value := range query {
			switch key {
			case "resolution":
				input += " -video_size " + value[0]
			case "pixel_format", "framerate", "video_size", "capture_cursor", "capture_mouse_clicks", "capture_raw_data":
				input += " -" + key + " " + value[0]
			}
		}
	}

	if audio != "" {
		audio = indexToItem(audios, audio)
	}

	return input + ` -i "` + video + `:` + audio + `"`
}

func initDevices() {
	// [AVFoundation indev @ 0x147f04510] AVFoundation video devices:
	// [AVFoundation indev @ 0x147f04510] [0] FaceTime HD Camera
	// [AVFoundation indev @ 0x147f04510] [1] Capture screen 0
	// [AVFoundation indev @ 0x147f04510] AVFoundation audio devices:
	// [AVFoundation indev @ 0x147f04510] [0] MacBook Pro Microphone
	cmd := exec.Command(
		Bin, "-hide_banner", "-list_devices", "true", "-f", "avfoundation", "-i", "",
	)
	b, _ := cmd.CombinedOutput()

	re := regexp.MustCompile(`\[(\d+)\] (.+?)(?:\s+\[uid:([^\]]+)\])?(?:\s+\[serial:([^\]]+)\])?\s*$`)

	var kind string
	for _, line := range strings.Split(string(b), "\n") {
		switch {
		case strings.HasSuffix(line, "video devices:"):
			kind = core.KindVideo
			continue
		case strings.HasSuffix(line, "audio devices:"):
			kind = core.KindAudio
			continue
		}

		m := re.FindStringSubmatch(line)
		if m == nil {
			continue
		}

		index := m[1]
		name := m[2]
		info := ""
		if m[3] != "" {
			info = "uid=" + m[3]
		}
		if m[4] != "" {
			if info != "" {
				info += ", "
			}
			info += "serial=" + m[4]
		}

		switch kind {
		case core.KindVideo:
			// Use the numeric index in generated URLs. The optional [uid:...]
			// suffix printed by FFmpeg is metadata, not part of the device name,
			// and passing it back causes "Video device not found".
			videos = append(videos, index)
		case core.KindAudio:
			audios = append(audios, index)
		}

		if kind == core.KindVideo {
			mode, modesInfo := probeVideoMode(index)
			videoModes[index] = mode
			videoModes[name] = mode
			if modesInfo != "" {
				if info != "" {
					info += "; "
				}
				info += modesInfo
			}
		}

		sourceURL := "ffmpeg:device?" + kind + "=" + index
		if kind == core.KindVideo {
			if mode := videoModes[index]; mode.Size != "" {
				sourceURL += "&video_size=" + mode.Size
			}
			if mode := videoModes[index]; mode.Framerate != "" {
				sourceURL += "&framerate=" + mode.Framerate
			}
		}

		streams = append(streams, &api.Source{
			Name: name, Info: info, URL: sourceURL,
		})
	}
}

type avfoundationMode struct {
	Size      string
	Framerate string
	Width     int
	Height    int
	FPS       float64
}

var videoModes = map[string]avfoundationMode{}

func probeVideoMode(index string) (avfoundationMode, string) {
	// An invalid size makes AVFoundation print the complete list of supported
	// modes without opening a long-running capture session.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx,
		Bin, "-hide_banner", "-f", "avfoundation",
		"-video_size", "1x1", "-i", index+":", "-t", "0.1",
	)
	b, _ := cmd.CombinedOutput()

	modes := parseAVFoundationModes(b)
	if len(modes) == 0 {
		return avfoundationMode{}, ""
	}

	mode := selectAVFoundationMode(modes)
	return mode, formatAVFoundationModes(modes, mode)
}

func parseAVFoundationModes(b []byte) []avfoundationMode {
	re := regexp.MustCompile(`\s+(\d+)x(\d+)@(?:\[)?(\d+(?:\.\d+)?)(?:\s+[^\]]+)?(?:\])?fps`)
	seen := make(map[string]struct{})
	var modes []avfoundationMode

	for _, match := range re.FindAllSubmatch(b, -1) {
		width, _ := strconv.Atoi(string(match[1]))
		height, _ := strconv.Atoi(string(match[2]))
		fps, _ := strconv.ParseFloat(string(match[3]), 64)
		size := string(match[1]) + "x" + string(match[2])
		key := size + "@" + string(match[3])
		if width <= 0 || height <= 0 || fps <= 0 {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		modes = append(modes, avfoundationMode{
			Size: string(size), Width: width, Height: height, FPS: fps,
			Framerate: formatFPS(fps),
		})
	}

	return modes
}

func selectAVFoundationMode(modes []avfoundationMode) avfoundationMode {
	stable := make([]avfoundationMode, 0, len(modes))
	for _, mode := range modes {
		if mode.FPS >= 25 {
			stable = append(stable, mode)
		}
	}
	if len(stable) > 0 {
		modes = stable
	}

	sort.Slice(modes, func(i, j int) bool {
		left, right := modes[i], modes[j]
		leftArea := left.Width * left.Height
		rightArea := right.Width * right.Height
		if leftArea != rightArea {
			return leftArea > rightArea
		}
		if left.FPS != right.FPS {
			return left.FPS > right.FPS
		}
		return left.Size < right.Size
	})

	return modes[0]
}

func formatAVFoundationModes(modes []avfoundationMode, selected avfoundationMode) string {
	bySize := make(map[string][]float64)
	for _, mode := range modes {
		bySize[mode.Size] = append(bySize[mode.Size], mode.FPS)
	}

	sizes := make([]string, 0, len(bySize))
	for size := range bySize {
		sizes = append(sizes, size)
	}
	sort.Slice(sizes, func(i, j int) bool {
		return sizes[i] < sizes[j]
	})

	parts := make([]string, 0, len(sizes))
	for _, size := range sizes {
		fps := bySize[size]
		sort.Float64s(fps)
		parts = append(parts, size+"@"+formatFPSRange(fps))
	}

	return "selected=" + selected.Size + "@" + selected.Framerate + ", supported=" + strings.Join(parts, ",")
}

func formatFPSRange(fps []float64) string {
	if len(fps) == 0 {
		return ""
	}
	min, max := fps[0], fps[len(fps)-1]
	if min == max {
		return formatFPS(min)
	}
	return formatFPS(min) + "-" + formatFPS(max)
}

func formatFPS(fps float64) string {
	if math.Abs(fps-math.Round(fps)) < 0.01 {
		return strconv.Itoa(int(math.Round(fps)))
	}
	return strconv.FormatFloat(fps, 'f', -1, 64)
}
