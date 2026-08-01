package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/pkg/yaml"
)

type simulateInfo struct {
	BasePath          string              `json:"base_path"`
	ConfiguredStreams map[string][]string `json:"configured_streams"`
	FilesAPI          string              `json:"files_api"`
	Host              string              `json:"host"`
	LogAPI            string              `json:"log_api"`
	ONVIFPath         string              `json:"onvif_path"`
	RTSPPath          string              `json:"rtsp_path"`
	RTSPPort          string              `json:"rtsp_port"`
	StreamsAPI        string              `json:"streams_api"`
	UploadAPI         string              `json:"upload_api"`
	UploadDir         string              `json:"upload_dir"`
}

func simulateHandler(w http.ResponseWriter, r *http.Request) {
	ResponseJSON(w, &simulateInfo{
		BasePath:          basePath,
		ConfiguredStreams: configuredStreamsFromFile(app.ConfigPath),
		FilesAPI:          simulateEndpoint("api/simulate/files"),
		Host:              r.Host,
		LogAPI:            simulateEndpoint("api/log"),
		ONVIFPath:         simulateEndpoint("onvif/device_service"),
		RTSPPath:          "/",
		RTSPPort:          "8554",
		StreamsAPI:        simulateEndpoint("api/streams"),
		UploadAPI:         simulateEndpoint("api/simulate/upload"),
		UploadDir:         filepath.ToSlash(simulateUploadDir),
	})
}

func simulateEndpoint(path string) string {
	return strings.TrimRight(basePath, "/") + "/" + strings.TrimLeft(path, "/")
}

func configuredStreamsFromFile(configPath string) map[string][]string {
	streams := map[string][]string{}
	if configPath == "" {
		return streams
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return streams
	}

	var cfg struct {
		Streams map[string]any `yaml:"streams"`
	}
	if err = yaml.Unmarshal(data, &cfg); err != nil {
		return streams
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

	return streams
}
