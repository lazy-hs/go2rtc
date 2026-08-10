package onvif

import (
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewEnvelopeWithHeaders(t *testing.T) {
	e := NewEnvelopeWithHeaders(
		url.UserPassword("admin", "secret"),
		`<wsa:Action xmlns:wsa="http://www.w3.org/2005/08/addressing">urn:test</wsa:Action>`,
	)
	e.Append(`<tds:GetCapabilities/>`)
	body := string(e.Bytes())

	require.Contains(t, body, `<s:Header>`)
	require.Contains(t, body, `<wsse:Username>admin</wsse:Username>`)
	require.Contains(t, body, `<wsa:Action`)
	require.Contains(t, body, `urn:test`)
	require.True(t, strings.Index(body, `<wsse:Security`) < strings.Index(body, `<wsa:Action`))
	require.True(t, strings.Index(body, `</s:Header>`) < strings.Index(body, `<s:Body>`))
}

func TestNewEnvelopeWithHeadersWithoutUser(t *testing.T) {
	e := NewEnvelopeWithHeaders(nil, `<test:Header xmlns:test="urn:test"/>`)
	e.Append(`<test:Body xmlns:test="urn:test"/>`)
	body := string(e.Bytes())

	require.Contains(t, body, `<s:Header><test:Header`)
	require.NotContains(t, body, `wsse:Security`)
}
