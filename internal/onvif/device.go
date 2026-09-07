package onvif

import (
	"net"
	"strings"
)

type deviceConfig struct {
	Name         string `yaml:"name"`
	Manufacturer string `yaml:"manufacturer"`
	Model        string `yaml:"model"`
	Firmware     string `yaml:"firmware"`
	Serial       string `yaml:"serial"`
	Hardware     string `yaml:"hardware"`
	MAC          string `yaml:"mac"`
}

var device = defaultDeviceConfig("")

func defaultDeviceConfig(firmware string) deviceConfig {
	return deviceConfig{
		Name:         "go2rtc",
		Manufacturer: "go2rtc",
		Model:        "go2rtc",
		Firmware:     firmware,
		Hardware:     "go2rtc",
		MAC:          defaultHardwareAddress(),
	}
}

func (c deviceConfig) withDefaults(firmware string) deviceConfig {
	defaults := defaultDeviceConfig(firmware)
	if c.Name == "" {
		c.Name = defaults.Name
	}
	if c.Manufacturer == "" {
		c.Manufacturer = defaults.Manufacturer
	}
	if c.Model == "" {
		c.Model = defaults.Model
	}
	if c.Firmware == "" {
		c.Firmware = defaults.Firmware
	}
	if c.Hardware == "" {
		c.Hardware = defaults.Hardware
	}
	if mac := normalizeHardwareAddress(c.MAC); mac != "" {
		c.MAC = mac
	} else {
		c.MAC = defaults.MAC
	}
	return c
}

func defaultHardwareAddress() string {
	interfaces, _ := net.Interfaces()
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagLoopback == 0 && len(iface.HardwareAddr) != 0 {
			return strings.ToUpper(iface.HardwareAddr.String())
		}
	}
	return ""
}

func normalizeHardwareAddress(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	mac, err := net.ParseMAC(value)
	if err != nil {
		return ""
	}
	return strings.ToUpper(mac.String())
}
