package onvif

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/AlexxIT/go2rtc/internal/app"
	pkgonvif "github.com/AlexxIT/go2rtc/pkg/onvif"
	"github.com/stretchr/testify/require"
)

func TestValidateONVIFAuthUsernameToken(t *testing.T) {
	body := onvifTestEnvelope(url.UserPassword("lazy", "Aa123456"), `<tds:GetDeviceInformation/>`)
	req := httptest.NewRequest(http.MethodPost, "/onvif/device_service", bytes.NewReader(body))

	require.True(t, validateONVIFAuth(req, body, rtspAuthConfig{Username: "lazy", Password: "Aa123456"}))
	require.False(t, validateONVIFAuth(req, body, rtspAuthConfig{Username: "lazy", Password: "Aa1234567"}))
	require.False(t, validateONVIFAuth(req, body, rtspAuthConfig{Username: "admin", Password: "Aa123456"}))
}

func TestValidateONVIFAuthPasswordText(t *testing.T) {
	body := []byte(`<?xml version="1.0" encoding="utf-8"?><s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Header><wsse:Security xmlns:wsse="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd"><wsse:UsernameToken><wsse:Username>lazy</wsse:Username><wsse:Password Type="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-username-token-profile-1.0#PasswordText">Aa123456</wsse:Password></wsse:UsernameToken></wsse:Security></s:Header><s:Body><tds:GetDeviceInformation xmlns:tds="http://www.onvif.org/ver10/device/wsdl"/></s:Body></s:Envelope>`)
	req := httptest.NewRequest(http.MethodPost, "/onvif/device_service", bytes.NewReader(body))

	require.True(t, validateONVIFAuth(req, body, rtspAuthConfig{Username: "lazy", Password: "Aa123456"}))
	require.False(t, validateONVIFAuth(req, body, rtspAuthConfig{Username: "lazy", Password: "Aa1234567"}))
}

func TestValidateONVIFAuthBasic(t *testing.T) {
	body := onvifTestEnvelope(nil, `<tds:GetDeviceInformation/>`)
	req := httptest.NewRequest(http.MethodPost, "/onvif/device_service", bytes.NewReader(body))
	req.SetBasicAuth("lazy", "Aa123456")

	require.True(t, validateONVIFAuth(req, body, rtspAuthConfig{Username: "lazy", Password: "Aa123456"}))
	require.False(t, validateONVIFAuth(req, body, rtspAuthConfig{Username: "lazy", Password: "Aa1234567"}))
}

func TestONVIFDeviceServiceRequiresAuthWhenConfigured(t *testing.T) {
	oldConfigPath := app.ConfigPath
	t.Cleanup(func() {
		app.ConfigPath = oldConfigPath
	})

	configPath := filepath.Join(t.TempDir(), "go2rtc.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`rtsp:
  username: lazy
  password: Aa123456
`), 0644))
	app.ConfigPath = configPath

	body := onvifTestEnvelope(nil, `<tds:GetDeviceInformation/>`)
	req := httptest.NewRequest(http.MethodPost, "/onvif/device_service", bytes.NewReader(body))
	res := httptest.NewRecorder()
	onvifDeviceService(res, req)
	require.Equal(t, http.StatusUnauthorized, res.Code)

	body = onvifTestEnvelope(url.UserPassword("lazy", "Aa123456"), `<tds:GetDeviceInformation/>`)
	req = httptest.NewRequest(http.MethodPost, "/onvif/device_service", bytes.NewReader(body))
	res = httptest.NewRecorder()
	onvifDeviceService(res, req)
	require.Equal(t, http.StatusOK, res.Code)

	body = onvifTestEnvelope(url.UserPassword("lazy", "Aa1234567"), `<tds:GetDeviceInformation/>`)
	req = httptest.NewRequest(http.MethodPost, "/onvif/device_service", bytes.NewReader(body))
	res = httptest.NewRecorder()
	onvifDeviceService(res, req)
	require.Equal(t, http.StatusUnauthorized, res.Code)
}

func TestONVIFDeviceServiceAllowsDateTimeWithoutAuth(t *testing.T) {
	oldConfigPath := app.ConfigPath
	t.Cleanup(func() {
		app.ConfigPath = oldConfigPath
	})

	configPath := filepath.Join(t.TempDir(), "go2rtc.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`rtsp:
  username: lazy
  password: Aa123456
`), 0644))
	app.ConfigPath = configPath

	body := onvifTestEnvelope(nil, `<tds:GetSystemDateAndTime/>`)
	req := httptest.NewRequest(http.MethodPost, "/onvif/device_service", bytes.NewReader(body))
	res := httptest.NewRecorder()
	onvifDeviceService(res, req)
	require.Equal(t, http.StatusOK, res.Code)
}

func onvifTestEnvelope(user *url.Userinfo, body string) []byte {
	e := pkgonvif.NewEnvelopeWithUser(user)
	e.Append(body)
	return e.Bytes()
}
