package onvif

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/pkg/yaml"
)

type simulateEventTemplateConfig struct {
	Enabled        bool   `json:"enabled" yaml:"enabled"`
	Topic          string `json:"topic" yaml:"topic"`
	SourceData     string `json:"sourceData,omitempty" yaml:"sourceData,omitempty"`
	StartData      string `json:"startData,omitempty" yaml:"startData,omitempty"`
	EndData        string `json:"endData,omitempty" yaml:"endData,omitempty"`
	StartOperation string `json:"startOperation,omitempty" yaml:"startOperation,omitempty"`
	EndOperation   string `json:"endOperation,omitempty" yaml:"endOperation,omitempty"`
}

type simulateEventConfig struct {
	Enabled   bool                          `json:"enabled" yaml:"enabled"`
	Interval  string                        `json:"interval" yaml:"interval"`
	Burst     int                           `json:"burst" yaml:"burst"`
	Permanent bool                          `json:"permanent" yaml:"permanent"`
	Templates []simulateEventTemplateConfig `json:"templates" yaml:"templates"`
}

func apiSimulateEvents(w http.ResponseWriter, r *http.Request) {
	if app.ConfigPath == "" {
		http.Error(w, "config file disabled", http.StatusGone)
		return
	}

	switch r.Method {
	case http.MethodGet:
		cfg, err := readSimulateEventConfig(app.ConfigPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		api.ResponseJSON(w, cfg)

	case http.MethodPut:
		var cfg simulateEventConfig
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&cfg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		normalizeSimulateEventConfig(&cfg)
		if err := app.PatchConfig([]string{"event", "enabled"}, cfg.Enabled); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := app.PatchConfig([]string{"event", "interval"}, cfg.Interval); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := app.PatchConfig([]string{"event", "burst"}, cfg.Burst); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := app.PatchConfig([]string{"event", "permanent"}, cfg.Permanent); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := app.PatchConfig([]string{"event", "templates"}, cfg.Templates); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		api.ResponseJSON(w, cfg)

	default:
		w.Header().Set("Allow", "GET, PUT")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func readSimulateEventConfig(path string) (simulateEventConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return simulateEventConfig{}, err
	}

	var document struct {
		Event eventConfig `yaml:"event"`
	}
	if err = yaml.Unmarshal(data, &document); err != nil {
		return simulateEventConfig{}, err
	}

	cfg := simulateEventConfig{
		Enabled:   document.Event.Enabled == nil || *document.Event.Enabled,
		Interval:  document.Event.Interval,
		Burst:     document.Event.Burst,
		Permanent: document.Event.Permanent,
		Templates: make([]simulateEventTemplateConfig, 0, len(document.Event.Templates)),
	}
	if cfg.Interval == "" {
		cfg.Interval = defaultEventInterval.String()
	}
	for _, template := range document.Event.Templates {
		cfg.Templates = append(cfg.Templates, simulateEventTemplateConfig{
			Enabled:        template.Enabled == nil || *template.Enabled,
			Topic:          template.Topic,
			SourceData:     template.SourceData,
			StartData:      template.StartData,
			EndData:        template.EndData,
			StartOperation: template.StartOperation,
			EndOperation:   template.EndOperation,
		})
	}
	return cfg, nil
}

func normalizeSimulateEventConfig(cfg *simulateEventConfig) {
	cfg.Interval = strings.TrimSpace(cfg.Interval)
	if cfg.Interval == "" {
		cfg.Interval = defaultEventInterval.String()
	}
	if cfg.Burst < 0 {
		cfg.Burst = 0
	}
	for i := range cfg.Templates {
		cfg.Templates[i].Topic = strings.TrimSpace(cfg.Templates[i].Topic)
	}
}
