package onvif

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/pkg/yaml"
	"github.com/stretchr/testify/require"
)

func TestSimulateEventTemplateCatalog(t *testing.T) {
	catalog := simulateEventTemplateCatalog()
	require.Len(t, catalog, 13)

	topics := make([]string, len(catalog))
	for i, template := range catalog {
		topics[i] = template.Topic
		require.True(t, template.Enabled)
		require.NotEmpty(t, template.Name)
		require.Equal(t, simulateEventStartData, template.StartData)
		require.Equal(t, simulateEventEndData, template.EndData)
		require.Equal(t, "Changed", template.StartOperation)
		require.Equal(t, "Deleted", template.EndOperation)
	}
	require.Equal(t, []string{
		"tns1:RuleEngine/FlameDetector",
		"tns1:RuleEngine/FieldDetector/ObjectsInside",
		"tns1:RuleEngine/LineDetector/Crossed",
		"tns1:RuleEngine/FieldDetector/Outside",
		"tns1:RuleEngine/RemovedObjectDetector",
		"tns1:RuleEngine/FaceDetector",
		"tns1:VideoSource/MotionAlarm",
		"tns1:RuleEngine/MyRuleDetector/PeopleDetect",
		"tns1:RuleEngine/MyRuleDetector/VehicleDetect",
		"tns1:RuleEngine/PackageDetector",
		"tns1:Device/HardwareFailure",
		"tns1:VideoSource/GlobalSceneChange",
		"tns1:RuleEngine/MyRuleDetector/ShockDetection",
	}, topics)
}

func TestSimulateEventConfigGetIncludesBackendCatalog(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "go2rtc.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`event:
  enabled: true
  interval: 10s
  burst: 2
  templates:
    - topic: "tns1:VideoSource/MotionAlarm"
      startData: '<tt:SimpleItem Value="true" Name="IsMotion"/>'
      endData: '<tt:SimpleItem Value="false" Name="IsMotion"/>'
`), 0o644))

	previousPath := app.ConfigPath
	app.ConfigPath = configPath
	t.Cleanup(func() { app.ConfigPath = previousPath })

	req := httptest.NewRequest(http.MethodGet, "/api/simulate/events", nil)
	res := httptest.NewRecorder()
	apiSimulateEvents(res, req)
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())

	var response simulateEventResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &response))
	require.True(t, response.Enabled)
	require.Equal(t, "10s", response.Interval)
	require.Len(t, response.Templates, 1)
	require.Len(t, response.Catalog, 13)
	require.Equal(t, "tns1:RuleEngine/FlameDetector", response.Catalog[0].Topic)
}

func TestReadSimulateEventConfigTemplateNames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "go2rtc.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`event:
  templates:
    #画面变化
    - topic: "tns1:VideoSource/MotionAlarm"
      startOperation: "Changed"
      endOperation: "Deleted"
    #区域闯入 (FieldIntrusion)
    - topic: "tns1:RuleEngine/FieldDetector/ObjectsInside"
`), 0o644))

	cfg, err := readSimulateEventConfig(path)
	require.NoError(t, err)
	require.Len(t, cfg.Templates, 2)
	require.Equal(t, "画面变化", cfg.Templates[0].Name)
	require.Equal(t, "区域闯入 (FieldIntrusion)", cfg.Templates[1].Name)
}

func TestNormalizeSimulateEventConfigFillsTemplateName(t *testing.T) {
	tests := []struct {
		topic string
		name  string
	}{
		{"tns1:RuleEngine/FlameDetector", "烟雾火焰"},
		{"tns1:RuleEngine/FieldDetector/ObjectsInside", "区域闯入"},
		{"tns1:RuleEngine/LineDetector/Crossed", "越线"},
		{"tns1:RuleEngine/FieldDetector/Outside", "区域离开"},
		{"tns1:RuleEngine/RemovedObjectDetector", "物品丢失"},
		{"tns1:RuleEngine/FaceDetector", "人脸检测"},
		{"tns1:VideoSource/MotionAlarm", "画面变化"},
		{"tns1:RuleEngine/MyRuleDetector/PeopleDetect", "人形检测"},
		{"tns1:RuleEngine/MyRuleDetector/VehicleDetect", "车辆检测"},
		{"tns1:RuleEngine/PackageDetector", "包裹检测"},
		{"tns1:Device/HardwareFailure", "设备故障"},
		{"tns1:VideoSource/GlobalSceneChange", "镜头遮挡"},
		{"tns1:RuleEngine/MyRuleDetector/ShockDetection", "设备移动"},
	}
	cfg := simulateEventConfig{Templates: make([]simulateEventTemplateConfig, len(tests))}
	for i, test := range tests {
		cfg.Templates[i].Topic = test.topic
	}

	normalizeSimulateEventConfig(&cfg)
	for i, test := range tests {
		require.Equal(t, test.name, cfg.Templates[i].Name)
	}
}

func TestSimulateEventConfigPutMergesCurrentTemplates(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "go2rtc.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`event:
  interval: "1m"
  permanent: true
  burst: 10
  templates:
    #画面变化
    - topic: "tns1:VideoSource/MotionAlarm"
      sourceData: '<tt:SimpleItem Value="analytics_video_source_audio_source" Name="VideoAnalyticsConfigurationToken"/><tt:SimpleItem Value="MyMotionDetectorRule" Name="Rule"/>'
      startData: '<tt:SimpleItem Value="true" Name="IsMotion"/>'
      endData: '<tt:SimpleItem Value="false" Name="IsMotion"/>'
      startOperation: "Changed"
      endOperation: "Deleted"
  #区域闯入 (FieldIntrusion)
    - topic: "tns1:RuleEngine/FieldDetector/ObjectsInside"
      startData: '<tt:SimpleItem Value="true" Name="IsMotion"/>'
      endData: '<tt:SimpleItem Value="false" Name="IsMotion"/>'
`), 0o644))

	previousPath := app.ConfigPath
	app.ConfigPath = configPath
	t.Cleanup(func() { app.ConfigPath = previousPath })

	body := bytes.NewBufferString(`{
  "enabled": true,
  "interval": "1m",
  "burst": 10,
  "permanent": true,
  "templates": [
    {"enabled": true, "name": "画面变化", "topic": "tns1:VideoSource/MotionAlarm"},
    {"enabled": true, "name": "区域闯入 (FieldIntrusion)", "topic": "tns1:RuleEngine/FieldDetector/ObjectsInside"},
    {"enabled": true, "name": "咳嗽声", "topic": "tns1:RuleEngine/AudioAnalytics/Cough", "startOperation": "Changed"}
  ]
}`)
	req := httptest.NewRequest(http.MethodPut, "/api/simulate/events", body)
	res := httptest.NewRecorder()

	apiSimulateEvents(res, req)
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())

	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	var document struct {
		Event simulateEventConfig `yaml:"event"`
	}
	require.NoError(t, yaml.Unmarshal(data, &document))
	require.Len(t, document.Event.Templates, 3)
	require.Equal(t, "画面变化", document.Event.Templates[0].Name)
	require.Contains(t, document.Event.Templates[0].SourceData, "VideoAnalyticsConfigurationToken")
	require.Equal(t, "区域闯入 (FieldIntrusion)", document.Event.Templates[1].Name)
	require.Equal(t, "咳嗽声", document.Event.Templates[2].Name)
}
