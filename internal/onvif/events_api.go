package onvif

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/pkg/yaml"
	yamlv3 "gopkg.in/yaml.v3"
)

type simulateEventTemplateConfig struct {
	Enabled        bool   `json:"enabled" yaml:"enabled"`
	Name           string `json:"name,omitempty" yaml:"name,omitempty"`
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
		current, err := readSimulateEventConfig(app.ConfigPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		cfg = mergeSimulateEventConfig(current, cfg)
		normalizeSimulateEventConfig(&cfg)
		if err := app.PatchConfig([]string{"event"}, cfg); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		api.ResponseJSON(w, cfg)

	default:
		w.Header().Set("Allow", "GET, PUT")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func mergeSimulateEventConfig(current, next simulateEventConfig) simulateEventConfig {
	currentByTopic := make(map[string]simulateEventTemplateConfig, len(current.Templates))
	for _, template := range current.Templates {
		if template.Topic != "" {
			currentByTopic[template.Topic] = template
		}
	}

	for i := range next.Templates {
		currentTemplate, ok := currentByTopic[next.Templates[i].Topic]
		if !ok {
			continue
		}
		mergeSimulateEventTemplateConfig(&next.Templates[i], currentTemplate)
	}
	return next
}

func mergeSimulateEventTemplateConfig(next *simulateEventTemplateConfig, current simulateEventTemplateConfig) {
	if strings.TrimSpace(next.Name) == "" {
		next.Name = current.Name
	}
	if strings.TrimSpace(next.SourceData) == "" {
		next.SourceData = current.SourceData
	}
	if strings.TrimSpace(next.StartData) == "" {
		next.StartData = current.StartData
	}
	if strings.TrimSpace(next.EndData) == "" {
		next.EndData = current.EndData
	}
	if strings.TrimSpace(next.StartOperation) == "" {
		next.StartOperation = current.StartOperation
	}
	if strings.TrimSpace(next.EndOperation) == "" {
		next.EndOperation = current.EndOperation
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
	templateNames := simulateEventTemplateNames(data)
	for _, template := range document.Event.Templates {
		cfg.Templates = append(cfg.Templates, simulateEventTemplateConfig{
			Enabled:        template.Enabled == nil || *template.Enabled,
			Name:           simulateEventTemplateName(templateNames[template.Topic], template.Topic),
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
		cfg.Templates[i].Name = strings.TrimSpace(cfg.Templates[i].Name)
		cfg.Templates[i].Topic = strings.TrimSpace(cfg.Templates[i].Topic)
		if cfg.Templates[i].Name == "" {
			cfg.Templates[i].Name = simulateEventTemplateName("", cfg.Templates[i].Topic)
		}
	}
}

func simulateEventTemplateNames(data []byte) map[string]string {
	var root yamlv3.Node
	if err := yamlv3.Unmarshal(data, &root); err != nil || len(root.Content) == 0 {
		return nil
	}

	event := simulateYAMLMappingValue(root.Content[0], "event")
	templates := simulateYAMLMappingValue(event, "templates")
	if templates == nil || templates.Kind != yamlv3.SequenceNode {
		return nil
	}

	names := make(map[string]string)
	for _, item := range templates.Content {
		if item == nil || item.Kind != yamlv3.MappingNode {
			continue
		}
		topic := simulateYAMLScalarValue(item, "topic")
		if topic == "" {
			continue
		}
		name := simulateEventCommentName(item.HeadComment)
		if name == "" {
			name = simulateYAMLScalarValue(item, "name")
		}
		if name != "" {
			names[topic] = name
		}
	}
	return names
}

func simulateYAMLMappingValue(node *yamlv3.Node, key string) *yamlv3.Node {
	if node == nil || node.Kind != yamlv3.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func simulateYAMLScalarValue(node *yamlv3.Node, key string) string {
	value := simulateYAMLMappingValue(node, key)
	if value == nil {
		return ""
	}
	return strings.TrimSpace(value.Value)
}

func simulateEventCommentName(comment string) string {
	for _, line := range strings.Split(comment, "\n") {
		name := strings.TrimSpace(line)
		name = strings.TrimPrefix(name, "#")
		name = strings.TrimSpace(name)
		if name != "" {
			return name
		}
	}
	return ""
}

func simulateEventTemplateName(name, topic string) string {
	if name = strings.TrimSpace(name); name != "" {
		return name
	}
	switch topic {
	case "tns1:VideoSource/MotionAlarm":
		return "画面变化"
	case "tns1:RuleEngine/PackageDetector":
		return "包裹检测"
	case "tns1:RuleEngine/MyRuleDetector/VehicleDetect":
		return "车辆检测"
	case "tns1:RuleEngine/FlameDetector":
		return "火焰检测"
	case "tns1:RuleEngine/FieldDetector/ObjectsInside":
		return "区域闯入"
	case "tns1:RuleEngine/LineDetector/Crossed":
		return "越线"
	case "tns1:RuleEngine/FieldDetector/Outside":
		return "区域离开"
	case "tns1:RuleEngine/AudioAnalytics/Cough":
		return "咳嗽声"
	case "tns1:RuleEngine/AudioAnalytics/Cry":
		return "哭声"
	default:
		return "自定义事件"
	}
}
