package onvif

import (
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	pkgonvif "github.com/AlexxIT/go2rtc/pkg/onvif"
)

const (
	discoveryGroupAddress  = "239.255.255.250:3702"
	discoveryProbeAction   = "http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe"
	discoveryMatchesAction = "http://schemas.xmlsoap.org/ws/2005/04/discovery/ProbeMatches"
)

type discoveryInterface struct {
	Index   int
	IP      net.IP
	Network *net.IPNet
}

var (
	discoveryInstanceID = uint64(time.Now().Unix())
	discoveryMessageNo  atomic.Uint64
)

func startDiscovery(apiPort int) {
	if apiPort <= 0 {
		log.Warn().Msg("[onvif] discovery disabled: API TCP listener is unavailable")
		return
	}

	endpointID := discoveryEndpointID(discoveryIdentitySeed())
	listeners, err := startDiscoveryListeners(discoveryGroupAddress, apiPort, endpointID)
	if err != nil {
		log.Warn().Err(err).Str("addr", discoveryGroupAddress).Msg("[onvif] discovery listen")
		return
	}
	log.Info().Str("addr", discoveryGroupAddress).Int("interfaces", len(listeners)).
		Msg("[onvif] discovery multicast listener started")
}

func startDiscoveryListeners(groupAddress string, apiPort int, endpointID string) ([]*net.UDPConn, error) {
	group, err := net.ResolveUDPAddr("udp4", groupAddress)
	if err != nil {
		return nil, err
	}
	interfaces := discoveryIPv4Interfaces()
	var listeners []*net.UDPConn
	var listenErrors []error
	for index, candidates := range groupDiscoveryInterfaces(interfaces) {
		iface, ifaceErr := net.InterfaceByIndex(index)
		if ifaceErr != nil || len(candidates) == 0 {
			continue
		}
		conn, listenErr := net.ListenMulticastUDP("udp4", iface, group)
		if listenErr != nil {
			log.Debug().Err(listenErr).Str("interface", iface.Name).Msg("[onvif] discovery join multicast")
			listenErrors = append(listenErrors, fmt.Errorf("interface %s: %w", iface.Name, listenErr))
			continue
		}
		_ = conn.SetReadBuffer(64 * 1024)
		listeners = append(listeners, conn)
		go serveDiscovery(conn, candidates, apiPort, endpointID)
	}

	if len(listeners) == 0 {
		if len(listenErrors) != 0 {
			return nil, errors.Join(listenErrors...)
		}
		return nil, errors.New("no multicast-capable IPv4 interface")
	}
	return listeners, nil
}

func serveDiscovery(conn *net.UDPConn, interfaces []discoveryInterface, apiPort int, endpointID string) {
	defer conn.Close()
	buffer := make([]byte, 64*1024)

	for {
		n, remote, err := conn.ReadFromUDP(buffer)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			log.Warn().Err(err).Msg("[onvif] discovery read")
			return
		}

		messageID, ok := parseDiscoveryProbe(buffer[:n])
		if !ok {
			continue
		}
		log.Info().Str("messageID", messageID).Stringer("remoteAddr", remote).Msg("收到 Probe 请求")

		localIP := selectDiscoveryIP(remote.IP, interfaces)
		if localIP == nil {
			log.Warn().Stringer("remoteAddr", remote).Msg("[onvif] no IPv4 interface for Probe response")
			continue
		}
		xaddr := fmt.Sprintf("http://%s/onvif/device_service", net.JoinHostPort(localIP.String(), fmt.Sprint(apiPort)))
		response := discoveryProbeMatchesResponse(messageID, endpointID, xaddr, discoveryMessageNo.Add(1))
		if _, err = conn.WriteToUDP(response, remote); err != nil {
			log.Warn().Err(err).Stringer("remoteAddr", remote).Msg("[onvif] send ProbeMatch")
			continue
		}
		log.Info().Str("messageID", messageID).Stringer("remoteAddr", remote).Msg("已回复 ProbeMatch")
	}
}

func groupDiscoveryInterfaces(interfaces []discoveryInterface) map[int][]discoveryInterface {
	groups := make(map[int][]discoveryInterface)
	for _, candidate := range interfaces {
		groups[candidate.Index] = append(groups[candidate.Index], candidate)
	}
	return groups
}

func parseDiscoveryProbe(request []byte) (string, bool) {
	if pkgonvif.GetRequestAction(request) != "Probe" {
		return "", false
	}
	action := strings.TrimSpace(pkgonvif.FindTagValue(request, "Action"))
	if action != "" && action != discoveryProbeAction {
		return "", false
	}
	messageID := strings.TrimSpace(pkgonvif.FindTagValue(request, "MessageID"))
	if messageID == "" {
		return "", false
	}
	types := strings.TrimSpace(pkgonvif.FindTagValue(request, "Types"))
	if types != "" && !strings.Contains(types, "NetworkVideoTransmitter") && !strings.Contains(types, ":Device") {
		return "", false
	}
	return messageID, true
}

func discoveryProbeMatchesResponse(relatesTo, endpointID, xaddr string, messageNumber uint64) []byte {
	return []byte(fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?><s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:wsa="http://schemas.xmlsoap.org/ws/2004/08/addressing" xmlns:d="http://schemas.xmlsoap.org/ws/2005/04/discovery" xmlns:dn="http://www.onvif.org/ver10/network/wsdl" xmlns:tds="http://www.onvif.org/ver10/device/wsdl"><s:Header><wsa:MessageID>urn:uuid:%s</wsa:MessageID><wsa:RelatesTo>%s</wsa:RelatesTo><wsa:To>http://schemas.xmlsoap.org/ws/2004/08/addressing/role/anonymous</wsa:To><wsa:Action>%s</wsa:Action><d:AppSequence InstanceId="%d" MessageNumber="%d"/></s:Header><s:Body><d:ProbeMatches><d:ProbeMatch><wsa:EndpointReference><wsa:Address>urn:uuid:%s</wsa:Address></wsa:EndpointReference><d:Types>dn:NetworkVideoTransmitter tds:Device</d:Types><d:Scopes>onvif://www.onvif.org/type/Network_Video_Transmitter onvif://www.onvif.org/type/video_encoder onvif://www.onvif.org/name/%s onvif://www.onvif.org/hardware/%s onvif://www.onvif.org/Profile/Streaming</d:Scopes><d:XAddrs>%s</d:XAddrs><d:MetadataVersion>1</d:MetadataVersion></d:ProbeMatch></d:ProbeMatches></s:Body></s:Envelope>`,
		pkgonvif.UUID(), escapeXML(relatesTo), discoveryMatchesAction, discoveryInstanceID, messageNumber, endpointID,
		url.PathEscape(device.Name), url.PathEscape(device.Hardware), escapeXML(xaddr)))
}

func discoveryIPv4Interfaces() []discoveryInterface {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	var result []discoveryInterface
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagMulticast == 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			ip, network := interfaceAddress(addr)
			if ip == nil || ip.To4() == nil || network == nil {
				continue
			}
			result = append(result, discoveryInterface{Index: iface.Index, IP: ip.To4(), Network: network})
		}
	}
	return result
}

func selectDiscoveryIP(remoteIP net.IP, interfaces []discoveryInterface) net.IP {
	for _, candidate := range interfaces {
		if candidate.Network.Contains(remoteIP) {
			return candidate.IP
		}
	}
	if len(interfaces) != 0 {
		return interfaces[0].IP
	}
	return nil
}

func discoveryIdentitySeed() string {
	interfaces, _ := net.Interfaces()
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagLoopback == 0 && len(iface.HardwareAddr) != 0 {
			return "go2rtc:" + strings.ToLower(iface.HardwareAddr.String())
		}
	}
	return "go2rtc"
}

func discoveryEndpointID(seed string) string {
	hash := sha1.Sum([]byte(seed))
	hash[6] = hash[6]&0x0f | 0x50
	hash[8] = hash[8]&0x3f | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		binary.BigEndian.Uint32(hash[0:4]),
		binary.BigEndian.Uint16(hash[4:6]),
		binary.BigEndian.Uint16(hash[6:8]),
		binary.BigEndian.Uint16(hash[8:10]),
		hash[10:16],
	)
}
