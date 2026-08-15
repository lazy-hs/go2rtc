package onvif

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCapabilitiesAdvertisePTZ(t *testing.T) {
	capabilities := string(GetCapabilitiesResponseWithQuery("camera.local:2984", "?channel=2"))
	require.Contains(t, capabilities, "http://camera.local:2984/onvif/ptz_service?channel=2")

	services := string(GetServicesResponseWithQuery("camera.local:2984", "?channel=2"))
	require.Contains(t, services, "http://www.onvif.org/ver20/ptz/wsdl")
	require.Contains(t, services, "http://camera.local:2984/onvif/ptz_service?channel=2")

	capabilities = string(GetCapabilitiesResponseWithQueryAndPTZ("camera.local:2984", "?channel=2", false))
	require.NotContains(t, capabilities, "/onvif/ptz_service")
	services = string(GetServicesResponseWithQueryAndPTZ("camera.local:2984", "?channel=2", false))
	require.NotContains(t, services, "<tds:Namespace>http://www.onvif.org/ver20/ptz/wsdl</tds:Namespace>")
	require.NotContains(t, services, "/onvif/ptz_service")
}

func TestProfileIncludesPTZConfiguration(t *testing.T) {
	response := string(GetProfileResponseWithProfile(Profile{
		Name: "main", Token: "main", SourceToken: "main", PTZToken: "main__ptz", PTZNode: "main__ptz_node",
	}))
	require.Contains(t, response, `<tt:PTZConfiguration token="main__ptz">`)
	require.Contains(t, response, `<tt:NodeToken>main__ptz_node</tt:NodeToken>`)
}

func TestPTZPresetsResponse(t *testing.T) {
	response := string(GetPTZPresetsResponse([]PTZPreset{{
		Token: "preset_1", Name: "Door", Position: PTZPosition{Pan: 0.25, Tilt: -0.5, Zoom: 0.75},
	}}))
	require.Contains(t, response, `token="preset_1"`)
	require.Contains(t, response, `<tt:Name>Door</tt:Name>`)
	require.Contains(t, response, `PanTilt x="0.250000" y="-0.500000"`)
	require.Contains(t, response, `Zoom x="0.750000"`)
}
