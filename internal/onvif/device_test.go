package onvif

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeviceConfigDefaults(t *testing.T) {
	configured := deviceConfig{Name: "Front Door", Model: "IPC-9000", Serial: "CAM-001"}.withDefaults("1.9.14")

	require.Equal(t, "Front Door", configured.Name)
	require.Equal(t, "go2rtc", configured.Manufacturer)
	require.Equal(t, "IPC-9000", configured.Model)
	require.Equal(t, "1.9.14", configured.Firmware)
	require.Equal(t, "CAM-001", configured.Serial)
	require.Equal(t, "go2rtc", configured.Hardware)
}

func TestConfiguredDeviceResponses(t *testing.T) {
	previousDevice := device
	device = deviceConfig{
		Name:         "Warehouse Camera",
		Manufacturer: "ACME",
		Model:        "IPC-9000",
		Firmware:     "2.1.0",
		Serial:       "CAM-001",
		Hardware:     "Virtual IPC",
	}
	defer func() { device = previousDevice }()

	information := callDeviceOperation(t, `<tds:GetDeviceInformation/>`)
	require.Contains(t, information, `<tds:Manufacturer>ACME</tds:Manufacturer>`)
	require.Contains(t, information, `<tds:Model>IPC-9000</tds:Model>`)
	require.Contains(t, information, `<tds:SerialNumber>CAM-001</tds:SerialNumber>`)

	scopes := callDeviceOperation(t, `<tds:GetScopes/>`)
	require.Contains(t, scopes, `onvif://www.onvif.org/name/Warehouse%20Camera`)
	require.Contains(t, scopes, `onvif://www.onvif.org/hardware/Virtual%20IPC`)
}

func callDeviceOperation(t *testing.T, operation string) string {
	t.Helper()
	body := `<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:tds="http://www.onvif.org/ver10/device/wsdl"><s:Body>` + operation + `</s:Body></s:Envelope>`
	request := httptest.NewRequest(http.MethodPost, "http://camera.local/onvif/device_service", strings.NewReader(body))
	recorder := httptest.NewRecorder()
	onvifDeviceService(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)
	return recorder.Body.String()
}
