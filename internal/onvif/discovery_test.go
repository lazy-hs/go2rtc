package onvif

import (
	"encoding/xml"
	"net"
	"strings"
	"testing"
	"time"

	pkgonvif "github.com/AlexxIT/go2rtc/pkg/onvif"
	"github.com/stretchr/testify/require"
)

func TestParseDiscoveryProbe(t *testing.T) {
	request := []byte(`<?xml version="1.0"?><s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:wsa="http://schemas.xmlsoap.org/ws/2004/08/addressing" xmlns:d="http://schemas.xmlsoap.org/ws/2005/04/discovery" xmlns:dn="http://www.onvif.org/ver10/network/wsdl"><s:Header><wsa:Action>http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe</wsa:Action><wsa:MessageID>uuid:test-message</wsa:MessageID></s:Header><s:Body><d:Probe><d:Types>dn:NetworkVideoTransmitter</d:Types></d:Probe></s:Body></s:Envelope>`)

	messageID, ok := parseDiscoveryProbe(request)
	require.True(t, ok)
	require.Equal(t, "uuid:test-message", messageID)

	notProbe := []byte(`<s:Envelope><s:Body><d:ProbeMatches/></s:Body></s:Envelope>`)
	_, ok = parseDiscoveryProbe(notProbe)
	require.False(t, ok)
}

func TestDiscoveryProbeMatchesResponse(t *testing.T) {
	previousDevice := device
	device = deviceConfig{Name: "Front Door Camera", Hardware: "Virtual IPC"}.withDefaults("1.0")
	defer func() { device = previousDevice }()

	endpointID := discoveryEndpointID("go2rtc:6c:1f:f7:aa:73:fc")
	body := discoveryProbeMatchesResponse("uuid:request-id", endpointID, "http://192.168.73.241:2984/onvif/device_service", 7)

	require.NoError(t, xml.Unmarshal(body, &struct{}{}))
	require.Equal(t, endpointID, discoveryEndpointID("go2rtc:6c:1f:f7:aa:73:fc"))
	require.Contains(t, string(body), discoveryMatchesAction)
	require.Contains(t, string(body), `<wsa:RelatesTo>uuid:request-id</wsa:RelatesTo>`)
	require.Contains(t, string(body), `<wsa:Address>urn:uuid:`+endpointID+`</wsa:Address>`)
	require.Contains(t, string(body), `<d:XAddrs>http://192.168.73.241:2984/onvif/device_service</d:XAddrs>`)
	require.Contains(t, string(body), `onvif://www.onvif.org/name/Front%20Door%20Camera`)
	require.Contains(t, string(body), `onvif://www.onvif.org/hardware/Virtual%20IPC`)
	require.Contains(t, string(body), `MessageNumber="7"`)
	require.Equal(t, 36, len(endpointID))
}

func TestSelectDiscoveryIP(t *testing.T) {
	_, lan, err := net.ParseCIDR("172.17.50.0/24")
	require.NoError(t, err)
	_, docker, err := net.ParseCIDR("172.18.0.0/16")
	require.NoError(t, err)
	interfaces := []discoveryInterface{
		{Index: 3, IP: net.ParseIP("172.17.50.190"), Network: lan},
		{Index: 8, IP: net.ParseIP("172.18.0.2"), Network: docker},
	}

	require.Equal(t, "172.18.0.2", selectDiscoveryIP(net.ParseIP("172.18.0.1"), interfaces).String())
	require.Equal(t, "172.17.50.190", selectDiscoveryIP(net.ParseIP("192.0.2.1"), interfaces).String())
}

func TestDiscoveryProbeResponseActionIsRecognizable(t *testing.T) {
	body := discoveryProbeMatchesResponse("uuid:request", discoveryEndpointID("go2rtc"), "http://127.0.0.1:2984/onvif/device_service", 1)
	require.Equal(t, "ProbeMatches", pkgonvif.GetRequestAction(body))
}

func TestDiscoveryUDPServer(t *testing.T) {
	interfaces := discoveryIPv4Interfaces()
	if len(interfaces) == 0 {
		t.Skip("no multicast-capable IPv4 interface")
	}

	listeners, err := startDiscoveryListeners("239.255.255.250:33702", 32984, discoveryEndpointID("go2rtc:test"))
	require.NoError(t, err)
	for _, listener := range listeners {
		require.NoError(t, listener.Close())
	}

	serverSocket, err := net.ListenUDP("udp4", &net.UDPAddr{IP: interfaces[0].IP, Port: 0})
	require.NoError(t, err)
	defer serverSocket.Close()
	go serveDiscovery(serverSocket, []discoveryInterface{interfaces[0]}, 32984, discoveryEndpointID("go2rtc:test"))

	probeSocket, err := net.ListenUDP("udp4", &net.UDPAddr{IP: interfaces[0].IP, Port: 0})
	require.NoError(t, err)
	defer probeSocket.Close()
	require.NoError(t, probeSocket.SetDeadline(time.Now().Add(3*time.Second)))

	messageID := "uuid:multicast-round-trip"
	request := []byte(`<?xml version="1.0"?><s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:wsa="http://schemas.xmlsoap.org/ws/2004/08/addressing" xmlns:d="http://schemas.xmlsoap.org/ws/2005/04/discovery" xmlns:dn="http://www.onvif.org/ver10/network/wsdl"><s:Header><wsa:Action>http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe</wsa:Action><wsa:MessageID>` + messageID + `</wsa:MessageID></s:Header><s:Body><d:Probe><d:Types>dn:NetworkVideoTransmitter</d:Types></d:Probe></s:Body></s:Envelope>`)
	_, err = probeSocket.WriteToUDP(request, serverSocket.LocalAddr().(*net.UDPAddr))
	require.NoError(t, err)

	buffer := make([]byte, 64*1024)
	for {
		n, _, readErr := probeSocket.ReadFromUDP(buffer)
		require.NoError(t, readErr)
		body := string(buffer[:n])
		if !strings.Contains(body, messageID) {
			continue
		}
		require.Contains(t, body, ":32984/onvif/device_service")
		return
	}
}
