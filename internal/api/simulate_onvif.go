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
	ServicePort  int    `json:"service_port" yaml:"-"`
	RTSPPort     int    `json:"rtsp_port" yaml:"-"`
	RTSPUsername string `json:"rtsp_username" yaml:"-"`
	RTSPPassword string `json:"rtsp_password" yaml:"-"`
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
		if cfg.ServicePort == 0 {
			cfg.ServicePort = current.ServicePort
		}
		if cfg.RTSPPort == 0 {
			cfg.RTSPPort = current.RTSPPort
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
		if cfg.ServicePort > 0 && cfg.ServicePort != current.ServicePort {
			if err = app.PatchConfig([]string{"api", "listen"}, fmt.Sprintf(":%d", cfg.ServicePort)); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		if cfg.RTSPPort > 0 && cfg.RTSPPort != current.RTSPPort {
			if err = app.PatchConfig([]string{"rtsp", "listen"}, fmt.Sprintf(":%d", cfg.RTSPPort)); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		if cfg.RTSPUsername != current.RTSPUsername {
			var value any = cfg.RTSPUsername
			if cfg.RTSPUsername == "" {
				value = nil
			}
			if err = app.PatchConfig([]string{"rtsp", "username"}, value); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		if cfg.RTSPPassword != current.RTSPPassword {
			var value any = cfg.RTSPPassword
			if cfg.RTSPPassword == "" {
				value = nil
			}
			if err = app.PatchConfig([]string{"rtsp", "password"}, value); err != nil {
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
		API   struct {
			Listen *string `yaml:"listen"`
		} `yaml:"api"`
		RTSP struct {
			Listen   *string `yaml:"listen"`
			Username string  `yaml:"username"`
			Password string  `yaml:"password"`
		} `yaml:"rtsp"`
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return document.ONVIF, err
	}
	if err = yaml.Unmarshal(data, &document); err != nil {
		return document.ONVIF, err
	}
	document.ONVIF.ServicePort = simulateConfigListenPort(document.API.Listen, 1984)
	document.ONVIF.RTSPPort = simulateConfigListenPort(document.RTSP.Listen, 8554)
	document.ONVIF.RTSPUsername = document.RTSP.Username
	document.ONVIF.RTSPPassword = document.RTSP.Password
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
	cfg.RTSPUsername = strings.TrimSpace(cfg.RTSPUsername)
	cfg.RTSPPassword = strings.TrimSpace(cfg.RTSPPassword)
	if len(cfg.RTSPUsername) > 256 || len(cfg.RTSPPassword) > 256 {
		return fmt.Errorf("RTSP username and password must not exceed 256 characters")
	}
	if cfg.RTSPPassword != "" && cfg.RTSPUsername == "" {
		return fmt.Errorf("RTSP 用户名为空时不能单独配置密码")
	}
	if err := validateSimulatePort("ONVIF 服务端口", cfg.ServicePort); err != nil {
		return err
	}
	if err := validateSimulatePort("RTSP 端口", cfg.RTSPPort); err != nil {
		return err
	}
	return nil
}

func validateSimulatePort(label string, port int) error {
	if port == 0 {
		return nil
	}
	if port < 1 || port > 65535 {
		return fmt.Errorf("%s必须在 1-65535 之间", label)
	}
	return nil
}

func simulateConfigListenPort(listen *string, defaultPort int) int {
	if listen == nil || *listen == "" {
		return defaultPort
	}
	return simulateListenPortWithDefault(*listen, defaultPort)
}

func newSimulateONVIFConfigResponse(cfg simulateONVIFDeviceConfig) simulateONVIFConfigResponse {
	defaults := simulateONVIFDeviceConfig{
		Name:         "go2rtc",
		Manufacturer: "go2rtc",
		Model:        "go2rtc",
		Firmware:     app.Version,
		Hardware:     "go2rtc",
		ServicePort:  1984,
		RTSPPort:     8554,
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
	if effective.ServicePort == 0 {
		effective.ServicePort = defaults.ServicePort
	}
	if effective.RTSPPort == 0 {
		effective.RTSPPort = defaults.RTSPPort
	}
	return simulateONVIFConfigResponse{Config: cfg, Defaults: defaults, Effective: effective}
}
