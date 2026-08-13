package streams

import (
	"errors"
	"net/url"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/rs/zerolog"
	yamlv3 "gopkg.in/yaml.v3"
)

func Init() {
	var cfg struct {
		Streams  map[string]any    `yaml:"streams"`
		Publish  map[string]any    `yaml:"publish"`
		Preload  map[string]string `yaml:"preload"`
		Simulate struct {
			DisabledStreams []string `yaml:"disabled_streams"`
		} `yaml:"simulate"`
	}

	app.LoadConfig(&cfg)

	log = app.GetLogger("streams")
	streamsMu.Lock()
	streamOrder = streamOrderFromConfig(app.ConfigPath)
	disabledStreams = stringSet(cfg.Simulate.DisabledStreams)

	for name, item := range cfg.Streams {
		if disabledStreams[name] {
			continue
		}
		streams[name] = NewStream(item)
	}
	normalizeStreamOrderLocked()
	streamsMu.Unlock()

	api.HandleFunc("api/streams", apiStreams)
	api.HandleFunc("api/streams/state", apiStreamState)
	api.HandleFunc("api/streams.dot", apiStreamsDOT)
	api.HandleFunc("api/preload", apiPreload)
	api.HandleFunc("api/schemes", apiSchemes)

	if cfg.Publish == nil && cfg.Preload == nil {
		return
	}

	time.AfterFunc(time.Second, func() {
		// range for nil map is OK
		for name, dst := range cfg.Publish {
			if stream := Get(name); stream != nil {
				Publish(stream, dst)
			}
		}
		for name, rawQuery := range cfg.Preload {
			if err := AddPreload(name, rawQuery); err != nil {
				log.Error().Err(err).Caller().Send()
			}
		}
	})
}

func New(name string, sources ...string) (*Stream, error) {
	for _, source := range sources {
		if !HasProducer(source) {
			return nil, errors.New("streams: source not supported")
		}

		if err := Validate(source); err != nil {
			return nil, err
		}
	}

	stream := NewStream(sources)

	streamsMu.Lock()
	streams[name] = stream
	addStreamOrderLocked(name)
	streamsMu.Unlock()

	return stream, nil
}

func Patch(name string, source string) (*Stream, error) {
	streamsMu.Lock()
	defer streamsMu.Unlock()

	// check if source links to some stream name from go2rtc
	if u, err := url.Parse(source); err == nil && u.Scheme == "rtsp" && len(u.Path) > 1 {
		rtspName := u.Path[1:]
		if stream, ok := streams[rtspName]; ok {
			if streams[name] != stream {
				// link (alias) streams[name] to streams[rtspName]
				streams[name] = stream
			}
			addStreamOrderLocked(name)
			return stream, nil
		}
	}

	if stream, ok := streams[source]; ok {
		if name != source {
			// link (alias) streams[name] to streams[source]
			streams[name] = stream
		}
		addStreamOrderLocked(name)
		return stream, nil
	}

	// check if src has supported scheme
	if !HasProducer(source) {
		return nil, errors.New("streams: source not supported")
	}

	if err := Validate(source); err != nil {
		return nil, err
	}

	// check an existing stream with this name
	if stream, ok := streams[name]; ok {
		stream.SetSource(source)
		addStreamOrderLocked(name)
		return stream, nil
	}

	// create new stream with this name
	stream := NewStream(source)
	streams[name] = stream
	addStreamOrderLocked(name)
	return stream, nil
}

func GetOrPatch(query url.Values) (*Stream, error) {
	// check if src param exists
	source := query.Get("src")
	if source == "" {
		return nil, errors.New("streams: source empty")
	}

	// check if src is stream name
	if stream := Get(source); stream != nil {
		return stream, nil
	}

	// check if name param provided
	if name := query.Get("name"); name != "" {
		return Patch(name, source)
	}

	// return new stream with src as name
	return Patch(source, source)
}

var log zerolog.Logger

// streams map

var streams = map[string]*Stream{}
var streamsMu sync.Mutex
var streamOrder []string
var disabledStreams = map[string]bool{}

func stringSet(items []string) map[string]bool {
	set := make(map[string]bool, len(items))
	for _, item := range items {
		if item != "" {
			set[item] = true
		}
	}
	return set
}

func Get(name string) *Stream {
	streamsMu.Lock()
	defer streamsMu.Unlock()
	return streams[name]
}

func Delete(name string) {
	streamsMu.Lock()
	defer streamsMu.Unlock()
	delete(streams, name)
	delete(disabledStreams, name)
	for i, item := range streamOrder {
		if item == name {
			streamOrder = append(streamOrder[:i], streamOrder[i+1:]...)
			break
		}
	}
}

func Disable(name string) {
	streamsMu.Lock()
	stream := streams[name]
	delete(streams, name)
	disabledStreams[name] = true
	for i, item := range streamOrder {
		if item == name {
			streamOrder = append(streamOrder[:i], streamOrder[i+1:]...)
			break
		}
	}
	streamsMu.Unlock()

	if stream != nil {
		stream.Close()
	}
}

func Enable(name string, sources ...string) error {
	streamsMu.Lock()
	delete(disabledStreams, name)
	if _, ok := streams[name]; ok {
		streamsMu.Unlock()
		return nil
	}
	streamsMu.Unlock()

	stream, err := New(name, sources...)
	if err != nil {
		return err
	}
	_ = stream
	return nil
}

func IsDisabled(name string) bool {
	streamsMu.Lock()
	defer streamsMu.Unlock()
	return disabledStreams[name]
}

func DisabledNames() []string {
	streamsMu.Lock()
	defer streamsMu.Unlock()

	names := make([]string, 0, len(disabledStreams))
	for name := range disabledStreams {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func GetAllNames() []string {
	streamsMu.Lock()
	defer streamsMu.Unlock()
	normalizeStreamOrderLocked()
	return append([]string(nil), streamOrder...)
}

func addStreamOrderLocked(name string) {
	for _, item := range streamOrder {
		if item == name {
			return
		}
	}
	streamOrder = append(streamOrder, name)
}

func normalizeStreamOrderLocked() {
	ordered := make([]string, 0, len(streams))
	seen := make(map[string]struct{}, len(streams))
	for _, name := range streamOrder {
		if _, ok := streams[name]; !ok {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		ordered = append(ordered, name)
	}

	remaining := make([]string, 0, len(streams)-len(ordered))
	for name := range streams {
		if _, ok := seen[name]; !ok {
			remaining = append(remaining, name)
		}
	}
	sort.Strings(remaining)
	streamOrder = append(ordered, remaining...)
}

func streamOrderFromConfig(configPath string) []string {
	if configPath == "" {
		return nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil
	}

	var document yamlv3.Node
	if yamlv3.Unmarshal(data, &document) != nil || len(document.Content) == 0 {
		return nil
	}

	root := document.Content[0]
	if root.Kind != yamlv3.MappingNode {
		return nil
	}

	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value != "streams" {
			continue
		}

		mapping := root.Content[i+1]
		if mapping.Kind != yamlv3.MappingNode {
			return nil
		}

		order := make([]string, 0, len(mapping.Content)/2)
		for j := 0; j+1 < len(mapping.Content); j += 2 {
			order = append(order, mapping.Content[j].Value)
		}
		return order
	}

	return nil
}

func GetAllSources() map[string][]string {
	streamsMu.Lock()
	sources := make(map[string][]string, len(streams))
	for name, stream := range streams {
		sources[name] = stream.Sources()
	}
	streamsMu.Unlock()
	return sources
}
