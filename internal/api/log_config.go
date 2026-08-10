package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/rs/zerolog"
	"gopkg.in/yaml.v3"
)

var supportedLogLevels = []string{
	"trace",
	"debug",
	"info",
	"warn",
	"error",
	"fatal",
	"panic",
	"disabled",
}

var logModuleNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

var reservedLogKeys = map[string]struct{}{
	"level":  {},
	"format": {},
	"output": {},
	"time":   {},
}

type logLevelConfig struct {
	Level   string            `json:"level"`
	Modules map[string]string `json:"modules"`
}

type logConfigResponse struct {
	Levels  []string       `json:"levels"`
	Modules []string       `json:"modules"`
	Config  logLevelConfig `json:"config"`
}

func logConfigHandler(w http.ResponseWriter, r *http.Request) {
	if app.ConfigPath == "" {
		http.Error(w, "config file disabled", http.StatusGone)
		return
	}

	switch r.Method {
	case http.MethodGet:
		cfg, err := readLogConfig(app.ConfigPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		ResponseJSON(w, newLogConfigResponse(cfg))

	case http.MethodPut:
		var cfg logLevelConfig
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&cfg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err := normalizeAndValidateLogConfig(&cfg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		current, err := readLogConfig(app.ConfigPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if current.Level != cfg.Level {
			if err = app.PatchConfig([]string{"log", "level"}, cfg.Level); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		moduleNames := make([]string, 0, len(cfg.Modules))
		for module := range cfg.Modules {
			moduleNames = append(moduleNames, module)
		}
		sort.Strings(moduleNames)
		for _, module := range moduleNames {
			level := cfg.Modules[module]
			if current.Modules[module] == level {
				continue
			}
			if err = patchOptionalLogLevel(module, level); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		ResponseJSON(w, newLogConfigResponse(cfg))

	default:
		w.Header().Set("Allow", "GET, PUT")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func readLogConfig(path string) (logLevelConfig, error) {
	cfg := logLevelConfig{Level: "info", Modules: map[string]string{}}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}

	var document struct {
		Log map[string]string `yaml:"log"`
	}
	if err = yaml.Unmarshal(data, &document); err != nil {
		return cfg, err
	}
	for key, value := range document.Log {
		value = strings.ToLower(strings.TrimSpace(value))
		if key == "level" {
			if value != "" {
				cfg.Level = value
			}
			continue
		}
		if _, reserved := reservedLogKeys[key]; !reserved {
			cfg.Modules[key] = value
		}
	}
	return cfg, nil
}

func newLogConfigResponse(cfg logLevelConfig) logConfigResponse {
	moduleSet := make(map[string]struct{}, len(cfg.Modules))
	for _, module := range app.LogModules() {
		moduleSet[module] = struct{}{}
	}
	for module := range cfg.Modules {
		moduleSet[module] = struct{}{}
	}

	moduleNames := make([]string, 0, len(moduleSet))
	for module := range moduleSet {
		moduleNames = append(moduleNames, module)
	}
	sort.Strings(moduleNames)
	return logConfigResponse{
		Levels:  supportedLogLevels,
		Modules: moduleNames,
		Config:  cfg,
	}
}

func normalizeAndValidateLogConfig(cfg *logLevelConfig) error {
	cfg.Level = strings.ToLower(strings.TrimSpace(cfg.Level))
	if err := validateLogLevel(cfg.Level, false); err != nil {
		return fmt.Errorf("level: %w", err)
	}

	normalized := make(map[string]string, len(cfg.Modules))
	for module, level := range cfg.Modules {
		module = strings.ToLower(strings.TrimSpace(module))
		if !logModuleNamePattern.MatchString(module) {
			return fmt.Errorf("invalid log module: %s", module)
		}
		if _, reserved := reservedLogKeys[module]; reserved {
			return fmt.Errorf("reserved log module: %s", module)
		}
		level = strings.ToLower(strings.TrimSpace(level))
		if err := validateLogLevel(level, true); err != nil {
			return fmt.Errorf("%s: %w", module, err)
		}
		normalized[module] = level
	}
	cfg.Modules = normalized
	return nil
}

func validateLogLevel(level string, optional bool) error {
	if optional && level == "" {
		return nil
	}
	if _, err := zerolog.ParseLevel(level); err != nil {
		return err
	}
	for _, supported := range supportedLogLevels {
		if level == supported {
			return nil
		}
	}
	return fmt.Errorf("unsupported log level: %s", level)
}

func patchOptionalLogLevel(module, level string) error {
	if level == "" {
		return app.PatchConfig([]string{"log", module}, nil)
	}
	return app.PatchConfig([]string{"log", module}, level)
}
