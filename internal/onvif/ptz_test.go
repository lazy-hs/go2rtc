package onvif

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	pkgonvif "github.com/AlexxIT/go2rtc/pkg/onvif"
	"github.com/stretchr/testify/require"
)

func TestPTZManagerMovesAndClamps(t *testing.T) {
	manager := newPTZManager(map[string]ptzStreamConfig{"main": {}})

	pan, tilt, zoom := 2.0, -2.0, 1.5
	require.NoError(t, manager.absoluteMove("main", &pan, &tilt, &zoom))
	state, err := manager.snapshot("main")
	require.NoError(t, err)
	require.Equal(t, 1.0, state.Pan)
	require.Equal(t, -1.0, state.Tilt)
	require.Equal(t, 1.0, state.Zoom)

	pan, tilt, zoom = -0.5, 0.25, -0.4
	require.NoError(t, manager.relativeMove("main", &pan, &tilt, &zoom))
	state, err = manager.snapshot("main")
	require.NoError(t, err)
	require.InDelta(t, 0.5, state.Pan, 0.0001)
	require.InDelta(t, -0.75, state.Tilt, 0.0001)
	require.InDelta(t, 0.6, state.Zoom, 0.0001)
}

func TestPTZGlobalSwitch(t *testing.T) {
	manager := newPTZManager(map[string]ptzStreamConfig{"main": {}})
	require.True(t, manager.globalEnabled())
	require.True(t, manager.enabled("main"))

	manager.setGlobalEnabled(false)
	require.False(t, manager.globalEnabled())
	require.False(t, manager.enabled("main"))
	pan := 0.5
	require.Error(t, manager.absoluteMove("main", &pan, nil, nil))

	manager.setGlobalEnabled(true)
	require.True(t, manager.enabled("main"))
	require.NoError(t, manager.absoluteMove("main", &pan, nil, nil))
}

func TestPTZContinuousMoveAndStop(t *testing.T) {
	manager := newPTZManager(map[string]ptzStreamConfig{"main": {}})
	require.NoError(t, manager.continuousMove("main", 1, -1, 1, 0))
	time.Sleep(80 * time.Millisecond)
	require.NoError(t, manager.stop("main", true, true))

	state, err := manager.snapshot("main")
	require.NoError(t, err)
	require.Greater(t, state.Pan, 0.0)
	require.Less(t, state.Tilt, 0.0)
	require.Greater(t, state.Zoom, 0.0)
	require.Zero(t, state.PanVelocity)
	require.Zero(t, state.TiltVelocity)
	require.Zero(t, state.ZoomVelocity)
}

func TestPTZSOAPAbsoluteMoveAndStatus(t *testing.T) {
	previous := ptz
	ptz = newPTZManager(map[string]ptzStreamConfig{"main": {}})
	t.Cleanup(func() { ptz = previous })

	request := []byte(`<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:tptz="http://www.onvif.org/ver20/ptz/wsdl" xmlns:tt="http://www.onvif.org/ver10/schema"><s:Body><tptz:AbsoluteMove><tptz:ProfileToken>main</tptz:ProfileToken><tptz:Position><tt:PanTilt x="0.25" y="-0.5"/><tt:Zoom x="0.75"/></tptz:Position></tptz:AbsoluteMove></s:Body></s:Envelope>`)
	response, err := ptzResponse("/onvif/ptz_service", request, pkgonvif.PTZAbsoluteMove)
	require.NoError(t, err)
	require.Contains(t, string(response), "AbsoluteMoveResponse")

	statusRequest := []byte(`<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:tptz="http://www.onvif.org/ver20/ptz/wsdl"><s:Body><tptz:GetStatus><tptz:ProfileToken>main</tptz:ProfileToken></tptz:GetStatus></s:Body></s:Envelope>`)
	response, err = ptzResponse("/onvif/ptz_service", statusRequest, pkgonvif.PTZGetStatus)
	require.NoError(t, err)
	require.Contains(t, string(response), `PanTilt x="0.250000" y="-0.500000"`)
	require.Contains(t, string(response), `Zoom x="0.750000"`)
}

func TestParsePTZTimeout(t *testing.T) {
	require.Equal(t, 1500*time.Millisecond, parsePTZTimeout("PT1.5S"))
	require.Equal(t, 2*time.Second, parsePTZTimeout("2s"))
	require.Zero(t, parsePTZTimeout("invalid"))
}

func TestPTZPresetsAndHome(t *testing.T) {
	manager := newPTZManager(map[string]ptzStreamConfig{"main": {}})
	pan, tilt, zoom := 0.4, -0.3, 0.7
	require.NoError(t, manager.absoluteMove("main", &pan, &tilt, &zoom))
	token, err := manager.setPreset("main", "Door", "")
	require.NoError(t, err)
	require.Equal(t, "preset_1", token)

	pan, tilt, zoom = 0, 0, 0
	require.NoError(t, manager.absoluteMove("main", &pan, &tilt, &zoom))
	require.NoError(t, manager.gotoPreset("main", token))
	state, err := manager.snapshot("main")
	require.NoError(t, err)
	require.InDelta(t, 0.4, state.Pan, 0.0001)
	require.InDelta(t, -0.3, state.Tilt, 0.0001)
	require.InDelta(t, 0.7, state.Zoom, 0.0001)

	require.NoError(t, manager.setHome("main"))
	pan, tilt, zoom = -0.5, 0.5, 0.1
	require.NoError(t, manager.absoluteMove("main", &pan, &tilt, &zoom))
	require.NoError(t, manager.gotoHome("main"))
	state, err = manager.snapshot("main")
	require.NoError(t, err)
	require.InDelta(t, 0.4, state.Pan, 0.0001)

	require.NoError(t, manager.removePreset("main", token))
	presets, err := manager.presets("main")
	require.NoError(t, err)
	require.Empty(t, presets)
}

func TestPTZSOAPPresets(t *testing.T) {
	previous := ptz
	ptz = newPTZManager(map[string]ptzStreamConfig{"main": {}})
	t.Cleanup(func() { ptz = previous })

	getRequest := []byte(`<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:tptz="http://www.onvif.org/ver20/ptz/wsdl"><s:Body><tptz:GetPresets><tptz:ProfileToken>main</tptz:ProfileToken></tptz:GetPresets></s:Body></s:Envelope>`)
	require.True(t, isPTZRequest("/onvif/ptz_service", pkgonvif.GetRequestAction(getRequest)))
	response, err := ptzResponse("/onvif/ptz_service", getRequest, pkgonvif.PTZGetPresets)
	require.NoError(t, err)
	require.Contains(t, string(response), `<tptz:GetPresetsResponse></tptz:GetPresetsResponse>`)

	httpRequest := httptest.NewRequest(http.MethodPost, "/onvif/ptz_service", strings.NewReader(string(getRequest)))
	httpResponse := httptest.NewRecorder()
	onvifDeviceService(httpResponse, httpRequest)
	require.Equal(t, http.StatusOK, httpResponse.Code)
	require.Contains(t, httpResponse.Header().Get("Content-Type"), "application/soap+xml")
	require.Contains(t, httpResponse.Body.String(), `<tptz:GetPresetsResponse></tptz:GetPresetsResponse>`)

	setRequest := []byte(`<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:tptz="http://www.onvif.org/ver20/ptz/wsdl"><s:Body><tptz:SetPreset><tptz:ProfileToken>main</tptz:ProfileToken><tptz:PresetName>Door</tptz:PresetName></tptz:SetPreset></s:Body></s:Envelope>`)
	response, err = ptzResponse("/onvif/ptz_service", setRequest, pkgonvif.PTZSetPreset)
	require.NoError(t, err)
	require.Contains(t, string(response), `<tptz:PresetToken>preset_1</tptz:PresetToken>`)

	response, err = ptzResponse("/onvif/ptz_service", getRequest, pkgonvif.PTZGetPresets)
	require.NoError(t, err)
	require.Contains(t, string(response), `token="preset_1"`)
	require.Contains(t, string(response), `<tt:Name>Door</tt:Name>`)
}
