package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/AlexxIT/go2rtc/internal/app"
	"gopkg.in/yaml.v3"
)

type simulateONVIFDeviceConfig struct {
	Name         string `json:"name" yaml:"name"`
	Manufacturer string `json:"manufacturer" yaml:"manufacturer"`
	Model        string `json:"model" yaml:"model"`
	Firmware     string `json:"firmware" yaml:"firmware"`
	Serial       string `json:"serial" yaml:"serial"`
	Hardware     string `json:"hardware" yaml:"hardware"`
}

type simulateONVIFConfigResponse struct {
	Config    simulateONVIFDeviceConfig `json:"config"`
	Defaults  simulateONVIFDeviceConfig `json:"defaults"`
	Effective simulateONVIFDeviceConfig `json:"effective"`
}

var simulateONVIFConfigFields = []struct {
	name  string
	value func(simulateONVIFDeviceConfig) string
}{
	{"name", func(cfg simulateONVIFDeviceConfig) string { return cfg.Name }},
	{"manufacturer", func(cfg simulateONVIFDeviceConfig) string { return cfg.Manufacturer }},
	{"model", func(cfg simulateONVIFDeviceConfig) string { return cfg.Model }},
	{"firmware", func(cfg simulateONVIFDeviceConfig) string { return cfg.Firmware }},
	{"serial", func(cfg simulateONVIFDeviceConfig) string { return cfg.Serial }},
	{"hardware", func(cfg simulateONVIFDeviceConfig) string { return cfg.Hardware }},
}

func simulateONVIFConfigHandler(w http.ResponseWriter, r *http.Request) {
	if app.ConfigPath == "" {
		http.Error(w, "config file disabled", http.StatusGone)
		return
	}

	switch r.Method {
	case http.MethodGet:
		cfg, err := readSimulateONVIFConfig(app.ConfigPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		ResponseJSON(w, newSimulateONVIFConfigResponse(cfg))

	case http.MethodPut:
		var cfg simulateONVIFDeviceConfig
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&cfg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := normalizeSimulateONVIFConfig(&cfg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		current, err := readSimulateONVIFConfig(app.ConfigPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		for _, field := range simulateONVIFConfigFields {
			value := field.value(cfg)
			if field.value(current) == value {
				continue
			}
			var patchValue any = value
			if value == "" {
				patchValue = nil
			}
			if err = app.PatchConfig([]string{"onvif", field.name}, patchValue); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		ResponseJSON(w, newSimulateONVIFConfigResponse(cfg))

	default:
		w.Header().Set("Allow", "GET, PUT")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func readSimulateONVIFConfig(path string) (simulateONVIFDeviceConfig, error) {
	var document struct {
		ONVIF simulateONVIFDeviceConfig `yaml:"onvif"`
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return document.ONVIF, err
	}
	if err = yaml.Unmarshal(data, &document); err != nil {
		return document.ONVIF, err
	}
	return document.ONVIF, nil
}

func normalizeSimulateONVIFConfig(cfg *simulateONVIFDeviceConfig) error {
	values := []*string{
		&cfg.Name,
		&cfg.Manufacturer,
		&cfg.Model,
		&cfg.Firmware,
		&cfg.Serial,
		&cfg.Hardware,
	}
	for _, value := range values {
		*value = strings.TrimSpace(*value)
		if len(*value) > 256 {
			return fmt.Errorf("ONVIF device field must not exceed 256 characters")
		}
	}
	return nil
}

func newSimulateONVIFConfigResponse(cfg simulateONVIFDeviceConfig) simulateONVIFConfigResponse {
	defaults := simulateONVIFDeviceConfig{
		Name:         "go2rtc",
		Manufacturer: "go2rtc",
		Model:        "go2rtc",
		Firmware:     app.Version,
		Hardware:     "go2rtc",
	}
	effective := cfg
	if effective.Name == "" {
		effective.Name = defaults.Name
	}
	if effective.Manufacturer == "" {
		effective.Manufacturer = defaults.Manufacturer
	}
	if effective.Model == "" {
		effective.Model = defaults.Model
	}
	if effective.Firmware == "" {
		effective.Firmware = defaults.Firmware
	}
	if effective.Hardware == "" {
		effective.Hardware = defaults.Hardware
	}
	return simulateONVIFConfigResponse{Config: cfg, Defaults: defaults, Effective: effective}
}
