package app

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/AlexxIT/go2rtc/pkg/creds"
	"github.com/AlexxIT/go2rtc/pkg/yaml"
)

func LoadConfig(v any) {
	for _, data := range configs {
		if err := yaml.Unmarshal(data, v); err != nil {
			Logger.Warn().Err(err).Send()
		}
	}
}

var configMu sync.Mutex

func PatchConfig(path []string, value any) error {
	configPath := ConfigPath
	if len(path) > 0 && isStreamConfigKey(path[0]) {
		configPath = StreamConfigPathOrConfig()
	}
	if configPath == "" {
		return errors.New("config file disabled")
	}

	configMu.Lock()
	defer configMu.Unlock()

	// empty config is OK
	b, _ := os.ReadFile(configPath)

	b, err := yaml.Patch(b, path, value)
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, b, 0644)
}

type flagConfig []string

func (c *flagConfig) String() string {
	return strings.Join(*c, " ")
}

func (c *flagConfig) Set(value string) error {
	*c = append(*c, value)
	return nil
}

var configs [][]byte

func StreamConfigPathOrConfig() string {
	if StreamConfigPath != "" {
		return StreamConfigPath
	}
	return ConfigPath
}

func initConfig(confs flagConfig) {
	if confs == nil {
		confs = []string{"go2rtc.yaml", platformStreamConfig(runtime.GOOS)}
	}

	for _, conf := range confs {
		if len(conf) == 0 {
			continue
		}
		if conf[0] == '{' {
			// config as raw YAML or JSON
			configs = append(configs, []byte(conf))
		} else if data := parseConfString(conf); data != nil {
			configs = append(configs, data)
		} else {
			// config as file
			data, _ := os.ReadFile(conf)
			streamConfig := isPlatformStreamConfig(conf)
			if !streamConfig && ConfigPath != "" && configContainsStreamSettings(data) {
				streamConfig = true
			}

			if streamConfig {
				if StreamConfigPath == "" {
					StreamConfigPath = conf
				}
				if storage == nil {
					initStorage()
				}
			} else if ConfigPath == "" {
				ConfigPath = conf
				initStorage()
			}

			if data == nil {
				continue
			}

			loadEnv(data)
			data = creds.ReplaceVars(data)
			configs = append(configs, data)
		}
	}

	if ConfigPath != "" {
		if !filepath.IsAbs(ConfigPath) {
			if cwd, err := os.Getwd(); err == nil {
				ConfigPath = filepath.Join(cwd, ConfigPath)
			}
		}
		Info["config_path"] = ConfigPath
	}
	if StreamConfigPath != "" {
		if !filepath.IsAbs(StreamConfigPath) {
			if cwd, err := os.Getwd(); err == nil {
				StreamConfigPath = filepath.Join(cwd, StreamConfigPath)
			}
		}
		Info["stream_config_path"] = StreamConfigPath
	}
}

func platformStreamConfig(goos string) string {
	switch goos {
	case "linux":
		return "go2rtc_linux.yaml"
	case "windows":
		return "go2rtc_windows.yaml"
	case "darwin":
		return "go2rtc_mac.yaml"
	default:
		return ""
	}
}

func isPlatformStreamConfig(path string) bool {
	base := strings.TrimSuffix(strings.ToLower(filepath.Base(path)), filepath.Ext(path))
	switch base {
	case "go2rtc_linux", "go2rtc_windows", "go2rtc_mac", "go2rtc_darwin":
		return true
	default:
		return false
	}
}

func configContainsStreamSettings(data []byte) bool {
	if len(data) == 0 {
		return false
	}

	var root map[string]any
	if err := yaml.Unmarshal(data, &root); err != nil {
		return false
	}
	for _, key := range []string{"ffmpeg", "streams", "publish", "preload", "simulate"} {
		if _, ok := root[key]; ok {
			return true
		}
	}
	return false
}

func isStreamConfigKey(key string) bool {
	switch key {
	case "ffmpeg", "streams", "publish", "preload", "simulate":
		return true
	default:
		return false
	}
}

func parseConfString(s string) []byte {
	i := strings.IndexByte(s, '=')
	if i < 0 {
		return nil
	}

	items := strings.Split(s[:i], ".")
	if len(items) < 2 {
		return nil
	}

	// `log.level=trace` => `{log: {level: trace}}`
	var pre string
	var suf = s[i+1:]
	for _, item := range items {
		pre += "{" + item + ": "
		suf += "}"
	}

	return []byte(pre + suf)
}
