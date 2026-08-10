package onvif

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNetworkInterfaceFromAddresses(t *testing.T) {
	_, alias, err := net.ParseCIDR("192.168.73.200/24")
	require.NoError(t, err)
	alias.IP = net.ParseIP("192.168.73.200")

	_, network, err := net.ParseCIDR("192.168.73.241/24")
	require.NoError(t, err)
	network.IP = net.ParseIP("192.168.73.241")

	iface := net.Interface{
		Name:         "eth0",
		HardwareAddr: net.HardwareAddr{0x6c, 0x1f, 0xf7, 0xaa, 0x73, 0xfc},
		MTU:          1500,
	}
	item, matched := networkInterfaceFromAddresses(iface, []net.Addr{alias, network}, net.ParseIP("192.168.73.241"))
	require.True(t, matched)
	require.Equal(t, "eth0", item.Token)
	require.Equal(t, "6C:1F:F7:AA:73:FC", item.HWAddress)
	require.Equal(t, "192.168.73.241", item.IPv4)
	require.Equal(t, 24, item.PrefixLength)
	require.Equal(t, 1500, item.MTU)
}

func TestAddressIP(t *testing.T) {
	require.Equal(t, "192.168.73.241", addressIP("192.168.73.241:2984").String())
	require.Equal(t, "2001:db8::1", addressIP("[2001:db8::1]:2984").String())
	require.Nil(t, addressIP("camera.local:2984"))
}

func TestNetworkInterfacesForRequest(t *testing.T) {
	interfaces, err := net.Interfaces()
	require.NoError(t, err)

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 || len(iface.HardwareAddr) == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		require.NoError(t, err)
		for _, addr := range addrs {
			ip, _ := interfaceAddress(addr)
			if ip == nil || ip.To4() == nil {
				continue
			}

			request := httptest.NewRequest(http.MethodPost, "http://camera.local/onvif/device_service", nil)
			localAddr := &net.TCPAddr{IP: ip, Port: 2984}
			request = request.WithContext(context.WithValue(request.Context(), http.LocalAddrContextKey, localAddr))
			selected := networkInterfacesForRequest(request)
			require.Len(t, selected, 1)
			require.Equal(t, iface.Name, selected[0].Name)
			require.Equal(t, strings.ToUpper(iface.HardwareAddr.String()), selected[0].HWAddress)
			return
		}
	}

	t.Skip("no active non-loopback IPv4 interface with a hardware address")
}
