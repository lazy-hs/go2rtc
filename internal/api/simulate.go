package api

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/pkg/yaml"
	yamlv3 "gopkg.in/yaml.v3"
)

type simulateInfo struct {
	BasePath           string                                  `json:"base_path"`
	ConfiguredOrder    []string                                `json:"configured_order"`
	ConfiguredStreams  map[string][]string                     `json:"configured_streams"`
	DevicesAPI         string                                  `json:"devices_api"`
	FilesAPI           string                                  `json:"files_api"`
	FolderPickerAPI    string                                  `json:"folder_picker_api"`
	Host               string                                  `json:"host"`
	LogAPI             string                                  `json:"log_api"`
	MetricsAPI         string                                  `json:"metrics_api"`
	NativeFolderPicker bool                                    `json:"native_folder_picker"`
	ONVIFConfigAPI     string                                  `json:"onvif_config_api"`
	ONVIFPath          string                                  `json:"onvif_path"`
	PTZAPI             string                                  `json:"ptz_api"`
	PTZEnabled         bool                                    `json:"ptz_enabled"`
	RTSPPath           string                                  `json:"rtsp_path"`
	RTSPPort           string                                  `json:"rtsp_port"`
	StreamStateAPI     string                                  `json:"stream_state_api"`
	EventsAPI          string                                  `json:"events_api"`
	DisabledStreams    []string                                `json:"disabled_streams"`
	ONVIFQuality       map[string]simulateONVIFStreamQuality   `json:"onvif_quality,omitempty"`
	ONVIFQualities     map[string][]simulateONVIFStreamQuality `json:"onvif_qualities"`
	StreamsAPI         string                                  `json:"streams_api"`
	UploadAPI          string                                  `json:"upload_api"`
	UploadDir          string                                  `json:"upload_dir"`
	UploadLimit        int64                                   `json:"upload_limit"`
}

type simulateONVIFStreamQuality struct {
	Width  int `json:"width" yaml:"width"`
	Height int `json:"height" yaml:"height"`
}

func simulateHandler(w http.ResponseWriter, r *http.Request) {
	configuredStreams, configuredOrder := configuredStreamsFromFile(app.ConfigPath)
	ResponseJSON(w, &simulateInfo{
		BasePath:           basePath,
		ConfiguredOrder:    configuredOrder,
		ConfiguredStreams:  configuredStreams,
		DevicesAPI:         simulateEndpoint("api/ffmpeg/devices"),
		FilesAPI:           simulateEndpoint("api/simulate/files"),
		Host:               r.Host,
		LogAPI:             simulateEndpoint("api/log"),
		MetricsAPI:         simulateEndpoint("api/simulate/metrics"),
		FolderPickerAPI:    simulateEndpoint("api/simulate/folder-picker"),
		ONVIFConfigAPI:     simulateEndpoint("api/simulate/onvif"),
		ONVIFPath:          simulateEndpoint("onvif/device_service"),
		PTZAPI:             simulateEndpoint("api/simulate/ptz"),
		PTZEnabled:         configuredPTZEnabledFromFile(app.ConfigPath),
		RTSPPath:           "/",
		RTSPPort:           simulateRTSPPort(app.ConfigPath),
		StreamStateAPI:     simulateEndpoint("api/streams/state"),
		EventsAPI:          simulateEndpoint("api/simulate/events"),
		DisabledStreams:    configuredDisabledStreamsFromFile(app.ConfigPath),
		ONVIFQualities:     configuredONVIFQualitiesFromFile(app.ConfigPath),
		StreamsAPI:         simulateEndpoint("api/streams"),
		UploadAPI:          simulateEndpoint("api/simulate/upload"),
		UploadDir:          filepath.ToSlash(simulateUploadDir),
		UploadLimit:        simulateUploadLimit,
		NativeFolderPicker: simulateFolderPickerAvailable() && simulateLocalRequest(r),
	})
}

func configuredPTZEnabledFromFile(configPath string) bool {
	if configPath == "" {
		return false
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return false
	}
	var cfg struct {
		Simulate struct {
			PTZEnabled *bool          `yaml:"ptz_enabled"`
			PTZ        map[string]any `yaml:"ptz"`
		} `yaml:"simulate"`
	}
	if yaml.Unmarshal(data, &cfg) != nil {
		return false
	}
	if cfg.Simulate.PTZEnabled != nil {
		return *cfg.Simulate.PTZEnabled && len(cfg.Simulate.PTZ) > 0
	}
	return len(cfg.Simulate.PTZ) > 0
}

func configuredDisabledStreamsFromFile(configPath string) []string {
	if configPath == "" {
		return nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil
	}

	var cfg struct {
		Simulate struct {
			DisabledStreams []string `yaml:"disabled_streams"`
		} `yaml:"simulate"`
	}
	if yaml.Unmarshal(data, &cfg) != nil {
		return nil
	}

	return cfg.Simulate.DisabledStreams
}

func configuredONVIFQualitiesFromFile(configPath string) map[string][]simulateONVIFStreamQuality {
	qualities := map[string][]simulateONVIFStreamQuality{}
	if configPath == "" {
		return qualities
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return qualities
	}

	var cfg struct {
		Simulate struct {
			ONVIFQuality   map[string]simulateONVIFStreamQuality   `yaml:"onvif_quality"`
			ONVIFQualities map[string][]simulateONVIFStreamQuality `yaml:"onvif_qualities"`
		} `yaml:"simulate"`
	}
	if yaml.Unmarshal(data, &cfg) != nil {
		return qualities
	}
	for name, items := range cfg.Simulate.ONVIFQualities {
		if len(items) > 0 {
			qualities[name] = items
		}
	}
	for name, quality := range cfg.Simulate.ONVIFQuality {
		if _, ok := qualities[name]; !ok && (quality.Width > 0 || quality.Height > 0) {
			qualities[name] = []simulateONVIFStreamQuality{quality}
		}
	}
	return qualities
}

func simulateRTSPPort(configPath string) string {
	if listen, ok := simulateRuntimeRTSPListen(); ok {
		return simulateListenPort(listen)
	}

	listen := ":8554"
	if configPath != "" {
		data, err := os.ReadFile(configPath)
		if err == nil {
			var cfg struct {
				RTSP struct {
					Listen *string `yaml:"listen"`
				} `yaml:"rtsp"`
			}
			if yaml.Unmarshal(data, &cfg) == nil && cfg.RTSP.Listen != nil {
				listen = *cfg.RTSP.Listen
			}
		}
	}
	return simulateListenPort(listen)
}

func simulateRuntimeRTSPListen() (string, bool) {
	value, ok := app.Info["rtsp"]
	if !ok {
		return "", false
	}

	data, err := json.Marshal(value)
	if err != nil {
		return "", false
	}
	var cfg struct {
		Listen *string `json:"listen"`
	}
	if json.Unmarshal(data, &cfg) != nil || cfg.Listen == nil {
		return "", false
	}
	return *cfg.Listen, true
}

func simulateListenPort(listen string) string {
	if listen == "" {
		return ""
	}
	return strconv.Itoa(simulateListenPortWithDefault(listen, 8554))
}

func simulateListenPortWithDefault(listen string, defaultPort int) int {
	if listen == "" {
		return 0
	}
	if _, port, err := net.SplitHostPort(listen); err == nil {
		if value, err := strconv.Atoi(port); err == nil && value > 0 && value <= 65535 {
			return value
		}
	}
	if port := strings.TrimPrefix(listen, ":"); port != listen {
		if value, err := strconv.Atoi(port); err == nil && value > 0 && value <= 65535 {
			return value
		}
	}
	if value, err := strconv.Atoi(listen); err == nil && value > 0 && value <= 65535 {
		return value
	}
	return defaultPort
}

func simulateEndpoint(path string) string {
	return strings.TrimRight(basePath, "/") + "/" + strings.TrimLeft(path, "/")
}

func configuredStreamsFromFile(configPath string) (map[string][]string, []string) {
	streams := map[string][]string{}
	if configPath == "" {
		return streams, nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return streams, nil
	}

	var cfg struct {
		Streams map[string]any `yaml:"streams"`
	}
	if err = yaml.Unmarshal(data, &cfg); err != nil {
		return streams, nil
	}

	for name, value := range cfg.Streams {
		var sources []string
		switch value := value.(type) {
		case string:
			sources = append(sources, value)
		case []any:
			for _, item := range value {
				if source, ok := item.(string); ok {
					sources = append(sources, source)
				}
			}
		case []string:
			sources = append(sources, value...)
		}

		if len(sources) > 0 {
			streams[name] = sources
		}
	}

	return streams, configuredStreamOrder(data, streams)
}

func configuredStreamOrder(data []byte, streams map[string][]string) []string {
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

		order := make([]string, 0, len(streams))
		for j := 0; j+1 < len(mapping.Content); j += 2 {
			name := mapping.Content[j].Value
			if len(streams[name]) > 0 {
				order = append(order, name)
			}
		}
		return order
	}

	return nil
}
