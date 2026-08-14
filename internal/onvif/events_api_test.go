package onvif

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/pkg/yaml"
	"github.com/stretchr/testify/require"
)

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
	cfg := simulateEventConfig{
		Templates: []simulateEventTemplateConfig{{
			Topic: "tns1:RuleEngine/PackageDetector",
		}},
	}

	normalizeSimulateEventConfig(&cfg)
	require.Equal(t, "包裹检测", cfg.Templates[0].Name)
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
