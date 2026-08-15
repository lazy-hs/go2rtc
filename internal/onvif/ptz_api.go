package onvif

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/streams"
)

type simulatePTZRequest struct {
	Action   string   `json:"action"`
	Enabled  *bool    `json:"enabled,omitempty"`
	Pan      *float64 `json:"pan,omitempty"`
	Tilt     *float64 `json:"tilt,omitempty"`
	Zoom     *float64 `json:"zoom,omitempty"`
	PanTilt  *bool    `json:"pan_tilt,omitempty"`
	StopZoom *bool    `json:"stop_zoom,omitempty"`
	Timeout  string   `json:"timeout,omitempty"`
}

type simulatePTZGlobalStatus struct {
	Enabled bool `json:"enabled"`
}

type simulatePTZStatus struct {
	Source       string  `json:"source"`
	Pan          float64 `json:"pan"`
	Tilt         float64 `json:"tilt"`
	Zoom         float64 `json:"zoom"`
	PanVelocity  float64 `json:"pan_velocity"`
	TiltVelocity float64 `json:"tilt_velocity"`
	ZoomVelocity float64 `json:"zoom_velocity"`
	MaxZoom      float64 `json:"max_zoom"`
}

func apiSimulatePTZ(w http.ResponseWriter, r *http.Request) {
	source := strings.TrimSpace(r.URL.Query().Get("src"))
	if source == "" {
		switch r.Method {
		case http.MethodGet:
			if r.URL.Query().Get("global") == "1" {
				api.ResponseJSON(w, simulatePTZGlobalStatus{Enabled: ptz.globalEnabled()})
				return
			}
			statuses := make([]simulatePTZStatus, 0, len(ptz.controllers))
			for _, name := range streams.GetAllNames() {
				if status, ok := simulatePTZStatusFor(name); ok {
					statuses = append(statuses, status)
				}
			}
			api.ResponseJSON(w, statuses)
		case http.MethodPut:
			var request simulatePTZRequest
			decoder := json.NewDecoder(r.Body)
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if request.Enabled == nil {
				http.Error(w, "PTZ enabled state required", http.StatusBadRequest)
				return
			}
			if app.ConfigPath != "" {
				if err := app.PatchConfig([]string{"simulate", "ptz_enabled"}, *request.Enabled); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
			}
			applyPTZGlobalEnabled(*request.Enabled)
			api.ResponseJSON(w, simulatePTZGlobalStatus{Enabled: ptz.globalEnabled()})
		default:
			w.Header().Set("Allow", "GET, PUT")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	if !ptz.enabled(source) {
		http.Error(w, "PTZ source is not enabled", http.StatusNotFound)
		return
	}
	if r.Method == http.MethodGet {
		status, _ := simulatePTZStatusFor(source)
		api.ResponseJSON(w, status)
		return
	}
	if r.Method != http.MethodPut {
		w.Header().Set("Allow", "GET, PUT")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request simulatePTZRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var err error
	switch strings.ToLower(strings.TrimSpace(request.Action)) {
	case "absolute":
		err = ptz.absoluteMove(source, request.Pan, request.Tilt, request.Zoom)
	case "relative":
		err = ptz.relativeMove(source, request.Pan, request.Tilt, request.Zoom)
	case "continuous":
		pan, tilt, zoom := floatValue(request.Pan), floatValue(request.Tilt), floatValue(request.Zoom)
		err = ptz.continuousMove(source, pan, tilt, zoom, parsePTZTimeout(request.Timeout))
	case "stop":
		panTilt, zoom := true, true
		if request.PanTilt != nil {
			panTilt = *request.PanTilt
		}
		if request.StopZoom != nil {
			zoom = *request.StopZoom
		}
		err = ptz.stop(source, panTilt, zoom)
	case "home":
		err = ptz.gotoHome(source)
	default:
		http.Error(w, "unsupported PTZ action", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	status, _ := simulatePTZStatusFor(source)
	api.ResponseJSON(w, status)
}

func simulatePTZStatusFor(source string) (simulatePTZStatus, bool) {
	state, err := ptz.snapshot(source)
	if err != nil {
		return simulatePTZStatus{}, false
	}
	return simulatePTZStatus{
		Source: source, Pan: state.Pan, Tilt: state.Tilt, Zoom: state.Zoom,
		PanVelocity: state.PanVelocity, TiltVelocity: state.TiltVelocity,
		ZoomVelocity: state.ZoomVelocity, MaxZoom: ptz.maxZoom(source),
	}, true
}

func floatValue(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}
