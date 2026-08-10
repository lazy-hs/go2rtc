package onvif

import (
	"net"
	"net/http"
	"strings"
	"sync"

	pkgonvif "github.com/AlexxIT/go2rtc/pkg/onvif"
)

var networkInterfaceLogOnce sync.Once

func networkInterfacesForRequest(r *http.Request) []pkgonvif.NetworkInterface {
	localIP := requestLocalIP(r)
	interfaces, err := net.Interfaces()
	if err != nil {
		log.Warn().Err(err).Msg("[onvif] get network interfaces")
		return nil
	}

	var fallback []pkgonvif.NetworkInterface
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 || len(iface.HardwareAddr) == 0 {
			continue
		}

		item, matches := onvifNetworkInterface(iface, localIP)
		if item == nil {
			continue
		}
		if matches {
			return []pkgonvif.NetworkInterface{*item}
		}
		fallback = append(fallback, *item)
	}

	return fallback
}

func logNetworkInterface(interfaces []pkgonvif.NetworkInterface) {
	if len(interfaces) == 0 {
		return
	}
	networkInterfaceLogOnce.Do(func() {
		iface := interfaces[0]
		log.Info().Str("name", iface.Name).Str("ip", iface.IPv4).Str("mac", iface.HWAddress).
			Msg("[onvif] network interface selected")
	})
}

func onvifNetworkInterface(iface net.Interface, localIP net.IP) (*pkgonvif.NetworkInterface, bool) {
	addrs, err := iface.Addrs()
	if err != nil {
		return nil, false
	}
	return networkInterfaceFromAddresses(iface, addrs, localIP)
}

func networkInterfaceFromAddresses(iface net.Interface, addrs []net.Addr, localIP net.IP) (*pkgonvif.NetworkInterface, bool) {
	item := &pkgonvif.NetworkInterface{
		Token:     iface.Name,
		Name:      iface.Name,
		HWAddress: strings.ToUpper(iface.HardwareAddr.String()),
		MTU:       iface.MTU,
	}
	matched := false

	for _, addr := range addrs {
		ip, ipNet := interfaceAddress(addr)
		if ip == nil {
			continue
		}
		if localIP != nil && localIP.Equal(ip) {
			matched = true
			if ip.To4() != nil {
				setNetworkInterfaceIPv4(item, ip, ipNet)
			}
		} else if item.IPv4 == "" && ip.To4() != nil {
			setNetworkInterfaceIPv4(item, ip, ipNet)
		}
	}

	if item.IPv4 == "" && !matched {
		return nil, false
	}
	return item, matched
}

func setNetworkInterfaceIPv4(item *pkgonvif.NetworkInterface, ip net.IP, network *net.IPNet) {
	item.IPv4 = ip.String()
	if network != nil {
		item.PrefixLength, _ = network.Mask.Size()
	}
}

func interfaceAddress(addr net.Addr) (net.IP, *net.IPNet) {
	switch value := addr.(type) {
	case *net.IPNet:
		return value.IP, value
	case *net.IPAddr:
		return value.IP, nil
	default:
		ip, network, err := net.ParseCIDR(addr.String())
		if err != nil {
			return nil, nil
		}
		return ip, network
	}
}

func requestLocalIP(r *http.Request) net.IP {
	if addr, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr); ok {
		if ip := addressIP(addr.String()); ip != nil && !ip.IsUnspecified() {
			return ip
		}
	}
	return addressIP(r.Host)
}

func addressIP(address string) net.IP {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = strings.Trim(address, "[]")
	}
	if zone := strings.LastIndexByte(host, '%'); zone >= 0 {
		host = host[:zone]
	}
	return net.ParseIP(host)
}
