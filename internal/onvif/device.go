package onvif

type deviceConfig struct {
	Name         string `yaml:"name"`
	Manufacturer string `yaml:"manufacturer"`
	Model        string `yaml:"model"`
	Firmware     string `yaml:"firmware"`
	Serial       string `yaml:"serial"`
	Hardware     string `yaml:"hardware"`
}

var device = defaultDeviceConfig("")

func defaultDeviceConfig(firmware string) deviceConfig {
	return deviceConfig{
		Name:         "go2rtc",
		Manufacturer: "go2rtc",
		Model:        "go2rtc",
		Firmware:     firmware,
		Hardware:     "go2rtc",
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
	return c
}
