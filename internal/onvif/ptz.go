package onvif

import (
	"encoding/xml"
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AlexxIT/go2rtc/internal/streams"
	pkgonvif "github.com/AlexxIT/go2rtc/pkg/onvif"
)

type ptzStreamConfig struct {
	Enabled   *bool   `yaml:"enabled"`
	MaxZoom   float64 `yaml:"max_zoom"`
	PanSpeed  float64 `yaml:"pan_speed"`
	TiltSpeed float64 `yaml:"tilt_speed"`
	ZoomSpeed float64 `yaml:"zoom_speed"`
	Home      struct {
		Pan  float64 `yaml:"pan"`
		Tilt float64 `yaml:"tilt"`
		Zoom float64 `yaml:"zoom"`
	} `yaml:"home"`
}

type ptzState struct {
	Pan          float64
	Tilt         float64
	Zoom         float64
	PanVelocity  float64
	TiltVelocity float64
	ZoomVelocity float64
}

type ptzController struct {
	mu         sync.Mutex
	state      ptzState
	home       ptzState
	panSpeed   float64
	tiltSpeed  float64
	zoomSpeed  float64
	maxZoom    float64
	updated    time.Time
	timerID    uint64
	presets    map[string]ptzPreset
	nextPreset int
}

type ptzPreset struct {
	Token string
	Name  string
	State ptzState
}

type ptzManager struct {
	controllers map[string]*ptzController
	active      atomic.Bool
}

var ptz = newPTZManager(nil)

// ptzPositionChanged is replaced by the video pipeline integration. Keeping
// the state layer independent lets ONVIF and the local API use the same model.
var ptzPositionChanged = func(string, ptzState) {}

func newPTZManager(configs map[string]ptzStreamConfig) *ptzManager {
	return newPTZManagerWithEnabled(configs, len(configs) > 0)
}

func newPTZManagerWithEnabled(configs map[string]ptzStreamConfig, enabled bool) *ptzManager {
	m := &ptzManager{controllers: make(map[string]*ptzController, len(configs))}
	for name, config := range configs {
		name = strings.TrimSpace(name)
		if name == "" || config.Enabled != nil && !*config.Enabled {
			continue
		}
		controller := &ptzController{
			panSpeed:  defaultPTZValue(config.PanSpeed, 0.6),
			tiltSpeed: defaultPTZValue(config.TiltSpeed, 0.6),
			zoomSpeed: defaultPTZValue(config.ZoomSpeed, 0.5),
			maxZoom:   defaultPTZValue(config.MaxZoom, 4),
			updated:   time.Now(),
			presets:   map[string]ptzPreset{},
		}
		controller.home = ptzState{
			Pan:  clamp(config.Home.Pan, -1, 1),
			Tilt: clamp(config.Home.Tilt, -1, 1),
			Zoom: clamp(config.Home.Zoom, 0, 1),
		}
		controller.state = controller.home
		m.controllers[name] = controller
	}
	m.active.Store(enabled && len(m.controllers) > 0)
	return m
}

func defaultPTZValue(value, fallback float64) float64 {
	if value <= 0 {
		return fallback
	}
	return value
}

func (m *ptzManager) enabled(source string) bool {
	if !m.globalEnabled() {
		return false
	}
	_, ok := m.controllers[source]
	return ok
}

func (m *ptzManager) globalEnabled() bool {
	return m != nil && m.active.Load()
}

func (m *ptzManager) setGlobalEnabled(enabled bool) {
	if m == nil {
		return
	}
	enabled = enabled && len(m.controllers) > 0
	m.active.Store(enabled)
	if enabled {
		return
	}
	for _, controller := range m.controllers {
		controller.mu.Lock()
		controller.advanceLocked(time.Now())
		controller.state.PanVelocity = 0
		controller.state.TiltVelocity = 0
		controller.state.ZoomVelocity = 0
		controller.timerID++
		controller.mu.Unlock()
	}
}

func (m *ptzManager) maxZoom(source string) float64 {
	controller, err := m.controller(source)
	if err != nil {
		return 4
	}
	return controller.maxZoom
}

func (m *ptzManager) controller(source string) (*ptzController, error) {
	if m != nil && m.globalEnabled() {
		if controller := m.controllers[source]; controller != nil {
			return controller, nil
		}
	}
	return nil, errors.New("ptz: source is not enabled")
}

func (m *ptzManager) snapshot(source string) (ptzState, error) {
	controller, err := m.controller(source)
	if err != nil {
		return ptzState{}, err
	}
	controller.mu.Lock()
	controller.advanceLocked(time.Now())
	state := controller.state
	controller.mu.Unlock()
	return state, nil
}

func (m *ptzManager) absoluteMove(source string, pan, tilt, zoom *float64) error {
	controller, err := m.controller(source)
	if err != nil {
		return err
	}
	controller.mu.Lock()
	controller.advanceLocked(time.Now())
	if pan != nil {
		controller.state.Pan = clamp(*pan, -1, 1)
	}
	if tilt != nil {
		controller.state.Tilt = clamp(*tilt, -1, 1)
	}
	if zoom != nil {
		controller.state.Zoom = clamp(*zoom, 0, 1)
	}
	controller.state.PanVelocity = 0
	controller.state.TiltVelocity = 0
	controller.state.ZoomVelocity = 0
	controller.timerID++
	state := controller.state
	controller.mu.Unlock()
	ptzPositionChanged(source, state)
	return nil
}

func (m *ptzManager) relativeMove(source string, pan, tilt, zoom *float64) error {
	controller, err := m.controller(source)
	if err != nil {
		return err
	}
	controller.mu.Lock()
	controller.advanceLocked(time.Now())
	if pan != nil {
		controller.state.Pan = clamp(controller.state.Pan+*pan, -1, 1)
	}
	if tilt != nil {
		controller.state.Tilt = clamp(controller.state.Tilt+*tilt, -1, 1)
	}
	if zoom != nil {
		controller.state.Zoom = clamp(controller.state.Zoom+*zoom, 0, 1)
	}
	controller.state.PanVelocity = 0
	controller.state.TiltVelocity = 0
	controller.state.ZoomVelocity = 0
	controller.timerID++
	state := controller.state
	controller.mu.Unlock()
	ptzPositionChanged(source, state)
	return nil
}

func (m *ptzManager) continuousMove(source string, pan, tilt, zoom float64, timeout time.Duration) error {
	controller, err := m.controller(source)
	if err != nil {
		return err
	}
	controller.mu.Lock()
	controller.advanceLocked(time.Now())
	controller.state.PanVelocity = clamp(pan, -1, 1)
	controller.state.TiltVelocity = clamp(tilt, -1, 1)
	controller.state.ZoomVelocity = clamp(zoom, -1, 1)
	controller.timerID++
	timerID := controller.timerID
	state := controller.state
	controller.mu.Unlock()
	ptzPositionChanged(source, state)
	if state.PanVelocity != 0 || state.TiltVelocity != 0 || state.ZoomVelocity != 0 {
		go runPTZContinuousMove(source, controller, timerID)
	}

	if timeout > 0 {
		time.AfterFunc(timeout, func() {
			controller.mu.Lock()
			if controller.timerID != timerID {
				controller.mu.Unlock()
				return
			}
			controller.advanceLocked(time.Now())
			controller.state.PanVelocity = 0
			controller.state.TiltVelocity = 0
			controller.state.ZoomVelocity = 0
			state := controller.state
			controller.mu.Unlock()
			ptzPositionChanged(source, state)
		})
	}
	return nil
}

func runPTZContinuousMove(source string, controller *ptzController, timerID uint64) {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for now := range ticker.C {
		controller.mu.Lock()
		if controller.timerID != timerID {
			controller.mu.Unlock()
			return
		}
		controller.advanceLocked(now)
		state := controller.state
		moving := state.PanVelocity != 0 || state.TiltVelocity != 0 || state.ZoomVelocity != 0
		controller.mu.Unlock()
		ptzPositionChanged(source, state)
		if !moving {
			return
		}
	}
}

func (m *ptzManager) stop(source string, panTilt, zoom bool) error {
	controller, err := m.controller(source)
	if err != nil {
		return err
	}
	controller.mu.Lock()
	controller.advanceLocked(time.Now())
	if panTilt {
		controller.state.PanVelocity = 0
		controller.state.TiltVelocity = 0
	}
	if zoom {
		controller.state.ZoomVelocity = 0
	}
	controller.timerID++
	state := controller.state
	controller.mu.Unlock()
	ptzPositionChanged(source, state)
	return nil
}

func (m *ptzManager) gotoHome(source string) error {
	controller, err := m.controller(source)
	if err != nil {
		return err
	}
	controller.mu.Lock()
	controller.state = controller.home
	controller.updated = time.Now()
	controller.timerID++
	state := controller.state
	controller.mu.Unlock()
	ptzPositionChanged(source, state)
	return nil
}

func (m *ptzManager) setHome(source string) error {
	controller, err := m.controller(source)
	if err != nil {
		return err
	}
	controller.mu.Lock()
	controller.advanceLocked(time.Now())
	controller.home = positionOnly(controller.state)
	controller.mu.Unlock()
	return nil
}

func (m *ptzManager) presets(source string) ([]ptzPreset, error) {
	controller, err := m.controller(source)
	if err != nil {
		return nil, err
	}
	controller.mu.Lock()
	presets := make([]ptzPreset, 0, len(controller.presets))
	for _, preset := range controller.presets {
		presets = append(presets, preset)
	}
	controller.mu.Unlock()
	sort.Slice(presets, func(i, j int) bool { return presets[i].Token < presets[j].Token })
	return presets, nil
}

func (m *ptzManager) setPreset(source, name, token string) (string, error) {
	controller, err := m.controller(source)
	if err != nil {
		return "", err
	}
	controller.mu.Lock()
	defer controller.mu.Unlock()
	controller.advanceLocked(time.Now())
	token = strings.TrimSpace(token)
	if token == "" {
		if len(controller.presets) >= 32 {
			return "", errors.New("ptz: maximum number of presets reached")
		}
		for {
			controller.nextPreset++
			token = "preset_" + strconv.Itoa(controller.nextPreset)
			if _, exists := controller.presets[token]; !exists {
				break
			}
		}
	} else if _, exists := controller.presets[token]; !exists && len(controller.presets) >= 32 {
		return "", errors.New("ptz: maximum number of presets reached")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		if current, ok := controller.presets[token]; ok && current.Name != "" {
			name = current.Name
		} else {
			name = token
		}
	}
	controller.presets[token] = ptzPreset{Token: token, Name: name, State: positionOnly(controller.state)}
	return token, nil
}

func (m *ptzManager) gotoPreset(source, token string) error {
	controller, err := m.controller(source)
	if err != nil {
		return err
	}
	controller.mu.Lock()
	preset, ok := controller.presets[strings.TrimSpace(token)]
	if !ok {
		controller.mu.Unlock()
		return errors.New("ptz: preset not found")
	}
	controller.state = positionOnly(preset.State)
	controller.updated = time.Now()
	controller.timerID++
	state := controller.state
	controller.mu.Unlock()
	ptzPositionChanged(source, state)
	return nil
}

func (m *ptzManager) removePreset(source, token string) error {
	controller, err := m.controller(source)
	if err != nil {
		return err
	}
	controller.mu.Lock()
	defer controller.mu.Unlock()
	token = strings.TrimSpace(token)
	if _, ok := controller.presets[token]; !ok {
		return errors.New("ptz: preset not found")
	}
	delete(controller.presets, token)
	return nil
}

func positionOnly(state ptzState) ptzState {
	return ptzState{Pan: state.Pan, Tilt: state.Tilt, Zoom: state.Zoom}
}

func (controller *ptzController) advanceLocked(now time.Time) {
	if controller.updated.IsZero() {
		controller.updated = now
		return
	}
	delta := now.Sub(controller.updated).Seconds()
	controller.updated = now
	if delta <= 0 {
		return
	}
	// Avoid large jumps after a machine suspend or debugger pause.
	delta = math.Min(delta, 0.25)
	controller.state.Pan = clamp(controller.state.Pan+controller.state.PanVelocity*controller.panSpeed*delta, -1, 1)
	controller.state.Tilt = clamp(controller.state.Tilt+controller.state.TiltVelocity*controller.tiltSpeed*delta, -1, 1)
	controller.state.Zoom = clamp(controller.state.Zoom+controller.state.ZoomVelocity*controller.zoomSpeed*delta, 0, 1)
}

func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func ptzConfigurationToken(source string) string {
	return source + "__ptz"
}

func ptzNodeToken(source string) string {
	return source + "__ptz_node"
}

func ptzSourceByConfiguration(token string) string {
	for source := range ptz.controllers {
		if ptzConfigurationToken(source) == token {
			return source
		}
	}
	return ""
}

func ptzSourceByNode(token string) string {
	for source := range ptz.controllers {
		if ptzNodeToken(source) == token {
			return source
		}
	}
	return ""
}

func ptzSourceByProfile(token string) string {
	if profile, ok := configuredONVIFProfile(token); ok && ptz.enabled(profile.SourceToken) {
		return profile.SourceToken
	}
	if ptz.enabled(token) {
		return token
	}
	return ""
}

func isPTZRequest(rPath, operation string) bool {
	if strings.Contains(strings.ToLower(rPath), "ptz") {
		return true
	}
	switch operation {
	case pkgonvif.PTZAbsoluteMove, pkgonvif.PTZContinuousMove, pkgonvif.PTZGetConfiguration,
		pkgonvif.PTZGetConfigurations, pkgonvif.PTZGetConfigurationOptions, pkgonvif.PTZGetNode,
		pkgonvif.PTZGetNodes, pkgonvif.PTZGetPresets, pkgonvif.PTZGetStatus, pkgonvif.PTZGotoHomePosition,
		pkgonvif.PTZGotoPreset, pkgonvif.PTZRelativeMove, pkgonvif.PTZRemovePreset,
		pkgonvif.PTZSetHomePosition, pkgonvif.PTZSetPreset, pkgonvif.PTZStop:
		return true
	default:
		return false
	}
}

type ptzVector struct {
	X float64 `xml:"x,attr"`
	Y float64 `xml:"y,attr"`
}

type ptzMoveRequest struct {
	Body struct {
		AbsoluteMove struct {
			Position struct {
				PanTilt *ptzVector `xml:"PanTilt"`
				Zoom    *ptzVector `xml:"Zoom"`
			} `xml:"Position"`
		} `xml:"AbsoluteMove"`
		RelativeMove struct {
			Translation struct {
				PanTilt *ptzVector `xml:"PanTilt"`
				Zoom    *ptzVector `xml:"Zoom"`
			} `xml:"Translation"`
		} `xml:"RelativeMove"`
		ContinuousMove struct {
			Velocity struct {
				PanTilt *ptzVector `xml:"PanTilt"`
				Zoom    *ptzVector `xml:"Zoom"`
			} `xml:"Velocity"`
			Timeout string `xml:"Timeout"`
		} `xml:"ContinuousMove"`
		Stop struct {
			PanTilt *bool `xml:"PanTilt"`
			Zoom    *bool `xml:"Zoom"`
		} `xml:"Stop"`
	} `xml:"Body"`
}

func ptzResponse(rPath string, request []byte, operation string) ([]byte, error) {
	switch operation {
	case pkgonvif.ServiceGetServiceCapabilities:
		return pkgonvif.GetPTZServiceCapabilitiesResponse(), nil
	case pkgonvif.PTZGetNodes:
		nodes := make([]pkgonvif.PTZNode, 0, len(ptz.controllers))
		for _, source := range streams.GetAllNames() {
			if ptz.enabled(source) {
				nodes = append(nodes, pkgonvif.PTZNode{Token: ptzNodeToken(source), Name: source + " PTZ"})
			}
		}
		return pkgonvif.GetPTZNodesResponse(nodes), nil
	case pkgonvif.PTZGetNode:
		source := ptzSourceByNode(pkgonvif.FindTagValue(request, "NodeToken"))
		if source == "" {
			return nil, errors.New("ptz: node not found")
		}
		return pkgonvif.GetPTZNodeResponse(pkgonvif.PTZNode{Token: ptzNodeToken(source), Name: source + " PTZ"}), nil
	case pkgonvif.PTZGetConfigurations:
		configs := make([]pkgonvif.PTZConfiguration, 0, len(ptz.controllers))
		for _, source := range streams.GetAllNames() {
			if ptz.enabled(source) {
				configs = append(configs, pkgonvif.PTZConfiguration{Token: ptzConfigurationToken(source), Name: source + " PTZ", NodeToken: ptzNodeToken(source)})
			}
		}
		return pkgonvif.GetPTZConfigurationsResponse(configs), nil
	case pkgonvif.PTZGetConfiguration:
		source := ptzSourceByConfiguration(pkgonvif.FindTagValue(request, "(?:PTZConfigurationToken|ConfigurationToken)"))
		if source == "" {
			return nil, errors.New("ptz: configuration not found")
		}
		return pkgonvif.GetPTZConfigurationResponse(pkgonvif.PTZConfiguration{Token: ptzConfigurationToken(source), Name: source + " PTZ", NodeToken: ptzNodeToken(source)}), nil
	case pkgonvif.PTZGetConfigurationOptions:
		return pkgonvif.GetPTZConfigurationOptionsResponse(), nil
	case pkgonvif.PTZGetStatus:
		source := ptzSourceByProfile(pkgonvif.FindTagValue(request, "ProfileToken"))
		state, err := ptz.snapshot(source)
		if err != nil {
			return nil, err
		}
		return pkgonvif.GetPTZStatusResponse(
			pkgonvif.PTZPosition{Pan: state.Pan, Tilt: state.Tilt, Zoom: state.Zoom},
			state.PanVelocity != 0 || state.TiltVelocity != 0, state.ZoomVelocity != 0,
		), nil
	case pkgonvif.PTZGetPresets:
		source := ptzSourceByProfile(pkgonvif.FindTagValue(request, "ProfileToken"))
		presets, err := ptz.presets(source)
		if err != nil {
			return nil, err
		}
		response := make([]pkgonvif.PTZPreset, 0, len(presets))
		for _, preset := range presets {
			response = append(response, pkgonvif.PTZPreset{
				Token: preset.Token, Name: preset.Name,
				Position: pkgonvif.PTZPosition{Pan: preset.State.Pan, Tilt: preset.State.Tilt, Zoom: preset.State.Zoom},
			})
		}
		return pkgonvif.GetPTZPresetsResponse(response), nil
	case pkgonvif.PTZSetPreset:
		source := ptzSourceByProfile(pkgonvif.FindTagValue(request, "ProfileToken"))
		if source == "" {
			return nil, errors.New("ptz: profile not found")
		}
		token, err := ptz.setPreset(source, pkgonvif.FindTagValue(request, "PresetName"), pkgonvif.FindTagValue(request, "PresetToken"))
		if err != nil {
			return nil, err
		}
		return pkgonvif.SetPTZPresetResponse(token), nil
	case pkgonvif.PTZAbsoluteMove, pkgonvif.PTZRelativeMove, pkgonvif.PTZContinuousMove, pkgonvif.PTZStop,
		pkgonvif.PTZGotoHomePosition, pkgonvif.PTZSetHomePosition, pkgonvif.PTZGotoPreset, pkgonvif.PTZRemovePreset:
		source := ptzSourceByProfile(pkgonvif.FindTagValue(request, "ProfileToken"))
		if source == "" {
			return nil, errors.New("ptz: profile not found")
		}
		var move ptzMoveRequest
		if err := xml.Unmarshal(request, &move); err != nil {
			return nil, err
		}
		switch operation {
		case pkgonvif.PTZAbsoluteMove:
			pan, tilt, zoom := vectorValues(move.Body.AbsoluteMove.Position.PanTilt, move.Body.AbsoluteMove.Position.Zoom)
			if err := ptz.absoluteMove(source, pan, tilt, zoom); err != nil {
				return nil, err
			}
		case pkgonvif.PTZRelativeMove:
			pan, tilt, zoom := vectorValues(move.Body.RelativeMove.Translation.PanTilt, move.Body.RelativeMove.Translation.Zoom)
			if err := ptz.relativeMove(source, pan, tilt, zoom); err != nil {
				return nil, err
			}
		case pkgonvif.PTZContinuousMove:
			panTilt := move.Body.ContinuousMove.Velocity.PanTilt
			zoomVector := move.Body.ContinuousMove.Velocity.Zoom
			var pan, tilt, zoom float64
			if panTilt != nil {
				pan, tilt = panTilt.X, panTilt.Y
			}
			if zoomVector != nil {
				zoom = zoomVector.X
			}
			if err := ptz.continuousMove(source, pan, tilt, zoom, parsePTZTimeout(move.Body.ContinuousMove.Timeout)); err != nil {
				return nil, err
			}
		case pkgonvif.PTZStop:
			panTilt, zoom := true, true
			if move.Body.Stop.PanTilt != nil {
				panTilt = *move.Body.Stop.PanTilt
			}
			if move.Body.Stop.Zoom != nil {
				zoom = *move.Body.Stop.Zoom
			}
			if err := ptz.stop(source, panTilt, zoom); err != nil {
				return nil, err
			}
		case pkgonvif.PTZGotoHomePosition:
			if err := ptz.gotoHome(source); err != nil {
				return nil, err
			}
		case pkgonvif.PTZSetHomePosition:
			if err := ptz.setHome(source); err != nil {
				return nil, err
			}
		case pkgonvif.PTZGotoPreset:
			if err := ptz.gotoPreset(source, pkgonvif.FindTagValue(request, "PresetToken")); err != nil {
				return nil, err
			}
		case pkgonvif.PTZRemovePreset:
			if err := ptz.removePreset(source, pkgonvif.FindTagValue(request, "PresetToken")); err != nil {
				return nil, err
			}
		}
		return pkgonvif.GetPTZMoveResponse(operation), nil
	default:
		return nil, errors.New("ptz: unsupported operation on " + rPath)
	}
}

func vectorValues(panTilt, zoom *ptzVector) (pan, tilt, zoomValue *float64) {
	if panTilt != nil {
		pan, tilt = &panTilt.X, &panTilt.Y
	}
	if zoom != nil {
		zoomValue = &zoom.X
	}
	return
}

func parsePTZTimeout(value string) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if strings.HasPrefix(value, "PT") && strings.HasSuffix(value, "S") {
		if seconds, err := time.ParseDuration(strings.TrimSuffix(strings.TrimPrefix(value, "PT"), "S") + "s"); err == nil {
			return seconds
		}
	}
	duration, _ := time.ParseDuration(value)
	return duration
}
