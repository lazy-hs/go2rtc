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
		RTSPPort:          simulateRTSPPort(app.ConfigPath),
		StreamsAPI:        simulateEndpoint("api/streams"),
		UploadAPI:         simulateEndpoint("api/simulate/upload"),
		UploadDir:         filepath.ToSlash(simulateUploadDir),
	})
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
	if _, port, err := net.SplitHostPort(listen); err == nil {
		return port
	}
	if port := strings.TrimPrefix(listen, ":"); port != listen {
		if value, err := strconv.Atoi(port); err == nil && value > 0 && value <= 65535 {
			return port
		}
	}
	if value, err := strconv.Atoi(listen); err == nil && value > 0 && value <= 65535 {
		return listen
	}
	return "8554"
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
