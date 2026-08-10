package onvif

import (
	"encoding/xml"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeviceInformationResponse(t *testing.T) {
	body := GetDeviceInformationResponse("ACME & Co", "IPC <Pro>", "2.1", "CAM-001", "HW-A")

	require.NoError(t, xml.Unmarshal(body, &struct{}{}))
	require.Contains(t, string(body), `<tds:Manufacturer>ACME &amp; Co</tds:Manufacturer>`)
	require.Contains(t, string(body), `<tds:Model>IPC &lt;Pro&gt;</tds:Model>`)
	require.Contains(t, string(body), `<tds:SerialNumber>CAM-001</tds:SerialNumber>`)
	require.Contains(t, string(body), `<tds:HardwareId>HW-A</tds:HardwareId>`)
}

func TestScopesResponse(t *testing.T) {
	body := GetScopesResponse("Front Door Camera", "Virtual IPC")

	require.NoError(t, xml.Unmarshal(body, &struct{}{}))
	require.Contains(t, string(body), `onvif://www.onvif.org/name/Front%20Door%20Camera`)
	require.Contains(t, string(body), `onvif://www.onvif.org/hardware/Virtual%20IPC`)
}
