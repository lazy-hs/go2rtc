package rtsp

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/go-zeromq/zmq4"
)

type onvifPTZState struct {
	Pan     float64
	Tilt    float64
	Zoom    float64
	MaxZoom float64
}

type onvifPTZStreamKey struct {
	Source string
	Width  int
	Height int
}

type onvifPTZStream struct {
	stream    *streams.Stream
	key       onvifPTZStreamKey
	endpoint  string
	updates   chan onvifPTZState
	applyFn   func(onvifPTZState) error
	done      chan struct{}
	closeOnce sync.Once
	mu        sync.Mutex
	socket    zmq4.Socket
}

const (
	onvifPTZApplyRetryInterval = 100 * time.Millisecond
	onvifPTZApplyMaxAttempts   = 20
)

var onvifPTZRegistry = struct {
	sync.Mutex
	enabled bool
	states  map[string]onvifPTZState
	streams map[onvifPTZStreamKey]*onvifPTZStream
}{
	enabled: true,
	states:  map[string]onvifPTZState{},
	streams: map[onvifPTZStreamKey]*onvifPTZStream{},
}

func SetONVIFPTZEnabled(enabled bool) {
	onvifPTZRegistry.Lock()
	if onvifPTZRegistry.enabled == enabled {
		onvifPTZRegistry.Unlock()
		return
	}
	onvifPTZRegistry.enabled = enabled
	var items []*onvifPTZStream
	if !enabled {
		items = make([]*onvifPTZStream, 0, len(onvifPTZRegistry.streams))
		for _, item := range onvifPTZRegistry.streams {
			items = append(items, item)
		}
		onvifPTZRegistry.streams = map[onvifPTZStreamKey]*onvifPTZStream{}
	}
	onvifPTZRegistry.Unlock()

	for _, item := range items {
		item.close()
	}
}

func ConfigureONVIFPTZ(source string, maxZoom, pan, tilt, zoom float64) {
	if maxZoom <= 1 {
		maxZoom = 4
	}
	UpdateONVIFPTZ(source, maxZoom, pan, tilt, zoom)
}

func UpdateONVIFPTZ(source string, maxZoom, pan, tilt, zoom float64) {
	state := onvifPTZState{Pan: pan, Tilt: tilt, Zoom: zoom, MaxZoom: maxZoom}
	onvifPTZRegistry.Lock()
	onvifPTZRegistry.states[source] = state
	items := make([]*onvifPTZStream, 0, len(onvifPTZRegistry.streams))
	for key, item := range onvifPTZRegistry.streams {
		if key.Source == source {
			items = append(items, item)
		}
	}
	onvifPTZRegistry.Unlock()

	for _, item := range items {
		item.stream.SetSource(item.sourceURL(state))
		item.queue(state)
	}
}

func onvifPTZStreamFor(name string, quality streamQuality) *streams.Stream {
	quality = normalizeRTSPQuality(quality)
	key := onvifPTZStreamKey{Source: name, Width: quality.Width, Height: quality.Height}

	onvifPTZRegistry.Lock()
	if !onvifPTZRegistry.enabled {
		onvifPTZRegistry.Unlock()
		return nil
	}
	if item := onvifPTZRegistry.streams[key]; item != nil {
		onvifPTZRegistry.Unlock()
		return item.stream
	}
	state, ok := onvifPTZRegistry.states[name]
	if !ok {
		onvifPTZRegistry.Unlock()
		return nil
	}
	endpoint, err := allocateONVIFPTZEndpoint()
	if err != nil {
		onvifPTZRegistry.Unlock()
		log.Error().Err(err).Str("stream", name).Msg("[rtsp] allocate ONVIF PTZ endpoint")
		return nil
	}

	item := &onvifPTZStream{
		key:      key,
		endpoint: endpoint,
		updates:  make(chan onvifPTZState, 1),
		done:     make(chan struct{}),
	}
	item.stream = streams.NewStream([]string{item.sourceURL(state)})
	onvifPTZRegistry.streams[key] = item
	onvifPTZRegistry.Unlock()
	go item.run()
	return item.stream
}

func ONVIFPTZStream(name string, width, height int) *streams.Stream {
	return onvifPTZStreamFor(name, streamQuality{Width: width, Height: height})
}

func (item *onvifPTZStream) sourceURL(state onvifPTZState) string {
	width, height := item.key.Width, item.key.Height
	if width <= 0 && height <= 0 {
		// The ONVIF server currently advertises the original profile as
		// 1920x1080. Keeping this output fixed prevents encoder/RTSP resets when
		// the crop window changes size at runtime.
		width, height = 1920, 1080
	}
	params := []string{
		"video=h264",
		"audio=copy",
		"ptz_fps=30",
		"ptz_endpoint=" + item.endpoint,
		"ptz_pan=" + formatPTZFloat(state.Pan),
		"ptz_tilt=" + formatPTZFloat(state.Tilt),
		"ptz_zoom=" + formatPTZFloat(state.Zoom),
		"ptz_max_zoom=" + formatPTZFloat(state.MaxZoom),
	}
	if width > 0 {
		params = append(params, "width="+strconv.Itoa(width))
	}
	if height > 0 {
		params = append(params, "height="+strconv.Itoa(height))
	}
	return "ffmpeg:" + item.key.Source + "#" + strings.Join(params, "#")
}

func allocateONVIFPTZEndpoint() (string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err = listener.Close(); err != nil {
		return "", err
	}
	return fmt.Sprintf("tcp://127.0.0.1:%d", port), nil
}

func formatPTZFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', 6, 64)
}

func (item *onvifPTZStream) queue(state onvifPTZState) {
	select {
	case <-item.done:
		return
	default:
	}
	select {
	case item.updates <- state:
	case <-item.done:
		return
	default:
		select {
		case <-item.updates:
		default:
		}
		select {
		case item.updates <- state:
		case <-item.done:
		default:
		}
	}
}

func (item *onvifPTZStream) run() {
	for {
		var state onvifPTZState
		select {
		case state = <-item.updates:
		case <-item.done:
			return
		}
		attempts := 0
		for {
			err := item.applyState(state)
			if err == nil {
				break
			}
			attempts++
			if attempts >= onvifPTZApplyMaxAttempts {
				log.Debug().Err(err).Str("endpoint", item.endpoint).Msg("[rtsp] ONVIF PTZ filter update")
				break
			}

			timer := time.NewTimer(onvifPTZApplyRetryInterval)
			select {
			case next := <-item.updates:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				state = next
				attempts = 0
			case <-timer.C:
			case <-item.done:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				return
			}
		}
	}
}

func (item *onvifPTZStream) close() {
	item.closeOnce.Do(func() {
		close(item.done)
		item.stream.Close()
		item.resetSocket()
	})
}

func (item *onvifPTZStream) applyState(state onvifPTZState) error {
	if item.applyFn != nil {
		return item.applyFn(state)
	}
	return item.apply(state)
}

func (item *onvifPTZStream) apply(state onvifPTZState) error {
	item.mu.Lock()
	defer item.mu.Unlock()

	if item.socket == nil {
		item.socket = zmq4.NewReq(context.Background(),
			zmq4.WithTimeout(100*time.Millisecond),
			zmq4.WithDialerTimeout(100*time.Millisecond),
			zmq4.WithDialerMaxRetries(0),
		)
		if err := item.socket.Dial(item.endpoint); err != nil {
			item.resetSocketLocked()
			return err
		}
	}

	factor := 1 + clampPTZ(state.Zoom, 0, 1)*(state.MaxZoom-1)
	x := (clampPTZ(state.Pan, -1, 1) + 1) / 2
	y := (1 - clampPTZ(state.Tilt, -1, 1)) / 2
	commands := []string{
		fmt.Sprintf("crop@ptz x (iw-ow)*%.6f", x),
		fmt.Sprintf("crop@ptz y (ih-oh)*%.6f", y),
		fmt.Sprintf("scale@ptz w floor(iw*%.6f/2)*2", factor),
	}
	for _, command := range commands {
		if err := item.socket.Send(zmq4.NewMsgString(command)); err != nil {
			item.resetSocketLocked()
			return err
		}
		response, err := item.socket.Recv()
		if err != nil {
			item.resetSocketLocked()
			return err
		}
		if text := string(response.Bytes()); !strings.HasPrefix(text, "0 ") && text != "0" {
			return fmt.Errorf("ffmpeg zmq: %s", text)
		}
	}
	return nil
}

func (item *onvifPTZStream) resetSocket() {
	item.mu.Lock()
	defer item.mu.Unlock()
	item.resetSocketLocked()
}

func (item *onvifPTZStream) resetSocketLocked() {
	if item.socket != nil {
		_ = item.socket.Close()
		item.socket = nil
	}
}

func clampPTZ(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
