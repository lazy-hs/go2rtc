package streams

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/creds"
	"github.com/AlexxIT/go2rtc/pkg/probe"
	"github.com/AlexxIT/go2rtc/pkg/yaml"
)

type onvifStreamQuality struct {
	Width  int `yaml:"width"`
	Height int `yaml:"height"`
}

func apiStreams(w http.ResponseWriter, r *http.Request) {
	w = creds.SecretResponse(w)

	query := r.URL.Query()
	src := query.Get("src")

	// without source - return all streams list
	if src == "" && r.Method != "POST" {
		api.ResponseJSON(w, streams)
		return
	}

	// Not sure about all this API. Should be rewrited...
	switch r.Method {
	case "GET":
		stream := Get(src)
		if stream == nil {
			http.Error(w, "", http.StatusNotFound)
			return
		}

		cons := probe.Create("probe", query)
		if len(cons.Medias) != 0 {
			cons.WithRequest(r)
			if err := stream.AddConsumer(cons); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			api.ResponsePrettyJSON(w, stream)

			stream.RemoveConsumer(cons)
		} else {
			api.ResponsePrettyJSON(w, streams[src])
		}

	case "PUT":
		name := query.Get("name")
		if name == "" {
			name = src
		}

		sources := query["src"]
		for _, source := range sources {
			if !HasProducer(source) {
				http.Error(w, "streams: source not supported", http.StatusBadRequest)
				return
			}
			if err := Validate(source); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}

		if err := app.PatchConfig([]string{"streams", name}, sources); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := patchONVIFStreamQualities(name, query); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if IsDisabled(name) {
			return
		}

		if _, err := New(name, sources...); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

	case "PATCH":
		name := query.Get("name")
		if name == "" {
			http.Error(w, "", http.StatusBadRequest)
			return
		}

		// support {input} templates: https://github.com/AlexxIT/go2rtc#module-hass
		if _, err := Patch(name, src); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}

	case "POST":
		// with dst - redirect source to dst
		if dst := query.Get("dst"); dst != "" {
			if stream := Get(dst); stream != nil {
				if err := Validate(src); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
				} else if err = stream.Play(src); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
				} else {
					api.ResponseJSON(w, stream)
				}
			} else if stream = Get(src); stream != nil {
				if err := Validate(dst); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
				} else if err = stream.Publish(dst); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
				}
			} else {
				http.Error(w, "", http.StatusNotFound)
			}
		} else {
			http.Error(w, "", http.StatusBadRequest)
		}

	case "DELETE":
		Delete(src)

		if err := app.PatchConfig([]string{"streams", src}, nil); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		disabled := stringSet(DisabledNames())
		delete(disabled, src)
		disabledNames := make([]string, 0, len(disabled))
		for disabledName := range disabled {
			disabledNames = append(disabledNames, disabledName)
		}
		sort.Strings(disabledNames)
		if err := app.PatchConfig([]string{"simulate", "disabled_streams"}, disabledNames); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := patchConfigIgnoreMissingPath([]string{"simulate", "onvif_quality", src}, nil); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := patchConfigIgnoreMissingPath([]string{"simulate", "onvif_qualities", src}, nil); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
	}
}

func patchONVIFStreamQualities(name string, query url.Values) error {
	values, ok := query["onvif_quality"]
	if !ok {
		return nil
	}

	qualities := make([]onvifStreamQuality, 0, len(values))
	seen := map[onvifStreamQuality]bool{}
	for _, item := range values {
		var quality onvifStreamQuality
		switch item {
		case "", "original":
			quality = onvifStreamQuality{}
		case "custom":
			quality = onvifStreamQuality{
				Width:  core.Atoi(query.Get("onvif_custom_width")),
				Height: core.Atoi(query.Get("onvif_custom_height")),
			}
		default:
			quality = onvifStreamQuality{Height: core.Atoi(item)}
		}
		if quality.Width < 0 || quality.Height < 0 {
			continue
		}
		if seen[quality] {
			continue
		}
		seen[quality] = true
		qualities = append(qualities, quality)
	}
	var value any = qualities
	if len(qualities) == 0 {
		value = nil
	}
	if err := app.PatchConfig([]string{"simulate", "onvif_qualities", name}, value); err != nil {
		if value != nil && isYAMLPathNotExist(err) {
			if err = app.PatchConfig([]string{"simulate", "onvif_qualities"}, map[string][]onvifStreamQuality{name: qualities}); err != nil {
				return err
			}
			return patchConfigIgnoreMissingPath([]string{"simulate", "onvif_quality", name}, nil)
		}
		return err
	}
	return patchConfigIgnoreMissingPath([]string{"simulate", "onvif_quality", name}, nil)
}

func patchConfigIgnoreMissingPath(path []string, value any) error {
	err := app.PatchConfig(path, value)
	if err != nil && value == nil && isYAMLPathNotExist(err) {
		return nil
	}
	return err
}

func isYAMLPathNotExist(err error) bool {
	return err != nil && strings.Contains(err.Error(), "yaml: path not exist")
}

type streamStateRequest struct {
	Enabled bool `json:"enabled"`
}

func apiStreamState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		w.Header().Set("Allow", "PUT")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name := r.URL.Query().Get("src")
	if name == "" {
		http.Error(w, "stream name required", http.StatusBadRequest)
		return
	}

	var req streamStateRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	configured := appConfiguredStreams()
	sources := configured[name]
	if len(sources) == 0 {
		http.Error(w, "stream config not found", http.StatusNotFound)
		return
	}

	disabled := stringSet(DisabledNames())
	if req.Enabled {
		delete(disabled, name)
	} else {
		disabled[name] = true
	}
	disabledNames := make([]string, 0, len(disabled))
	for disabledName := range disabled {
		disabledNames = append(disabledNames, disabledName)
	}
	sort.Strings(disabledNames)

	if err := app.PatchConfig([]string{"simulate", "disabled_streams"}, disabledNames); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Enabled {
		if err := Enable(name, sources...); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		Disable(name)
	}

	api.ResponseJSON(w, map[string]any{"name": name, "enabled": req.Enabled, "disabled_streams": disabledNames})
}

func appConfiguredStreams() map[string][]string {
	streams := map[string][]string{}
	if app.ConfigPath == "" {
		return streams
	}
	data, err := os.ReadFile(app.ConfigPath)
	if err != nil {
		return streams
	}

	var cfg struct {
		Streams map[string]any `yaml:"streams"`
	}
	if yaml.Unmarshal(data, &cfg) != nil {
		return streams
	}
	for name, value := range cfg.Streams {
		switch value := value.(type) {
		case string:
			streams[name] = []string{value}
		case []any:
			for _, item := range value {
				if source, ok := item.(string); ok {
					streams[name] = append(streams[name], source)
				}
			}
		case []string:
			streams[name] = append(streams[name], value...)
		}
	}
	return streams
}

func apiStreamsDOT(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	dot := make([]byte, 0, 1024)
	dot = append(dot, "digraph {\n"...)
	if query.Has("src") {
		for _, name := range query["src"] {
			if stream := streams[name]; stream != nil {
				dot = AppendDOT(dot, stream)
			}
		}
	} else {
		for _, stream := range streams {
			dot = AppendDOT(dot, stream)
		}
	}
	dot = append(dot, '}')

	dot = []byte(creds.SecretString(string(dot)))

	api.Response(w, dot, "text/vnd.graphviz")
}

func apiPreload(w http.ResponseWriter, r *http.Request) {
	// GET - return all preloads
	if r.Method == "GET" {
		api.ResponseJSON(w, GetPreloads())
		return
	}

	query := r.URL.Query()
	src := query.Get("src")

	switch r.Method {
	case "PUT":
		// it's safe to delete from map while iterating
		for k := range query {
			switch k {
			case core.KindVideo, core.KindAudio, "microphone":
			default:
				delete(query, k)
			}
		}

		rawQuery := query.Encode()

		if err := AddPreload(src, rawQuery); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if err := app.PatchConfig([]string{"preload", src}, rawQuery); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

	case "DELETE":
		if err := DelPreload(src); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if err := app.PatchConfig([]string{"preload", src}, nil); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

	default:
		http.Error(w, "", http.StatusMethodNotAllowed)
	}
}

func apiSchemes(w http.ResponseWriter, r *http.Request) {
	api.ResponseJSON(w, SupportedSchemes())
}
