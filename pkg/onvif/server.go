package onvif

import (
	"bytes"
	"encoding/xml"
	"net/url"
	"regexp"
	"time"
)

const ServiceGetServiceCapabilities = "GetServiceCapabilities"

const (
	DeviceGetCapabilities          = "GetCapabilities"
	DeviceGetDeviceInformation     = "GetDeviceInformation"
	DeviceGetDiscoveryMode         = "GetDiscoveryMode"
	DeviceGetDNS                   = "GetDNS"
	DeviceGetHostname              = "GetHostname"
	DeviceGetNetworkDefaultGateway = "GetNetworkDefaultGateway"
	DeviceGetNetworkInterfaces     = "GetNetworkInterfaces"
	DeviceGetNetworkProtocols      = "GetNetworkProtocols"
	DeviceGetNTP                   = "GetNTP"
	DeviceGetScopes                = "GetScopes"
	DeviceGetServices              = "GetServices"
	DeviceGetSystemDateAndTime     = "GetSystemDateAndTime"
	DeviceSetSystemDateAndTime     = "SetSystemDateAndTime"
	DeviceSystemReboot             = "SystemReboot"
)

const (
	MediaGetAudioEncoderConfigurations       = "GetAudioEncoderConfigurations"
	MediaGetAudioSources                     = "GetAudioSources"
	MediaGetAudioSourceConfigurations        = "GetAudioSourceConfigurations"
	MediaGetProfile                          = "GetProfile"
	MediaGetProfiles                         = "GetProfiles"
	MediaGetSnapshotUri                      = "GetSnapshotUri"
	MediaGetStreamUri                        = "GetStreamUri"
	MediaGetVideoEncoderConfiguration        = "GetVideoEncoderConfiguration"
	MediaGetVideoEncoderConfigurations       = "GetVideoEncoderConfigurations"
	MediaGetVideoEncoderConfigurationOptions = "GetVideoEncoderConfigurationOptions"
	MediaGetVideoSources                     = "GetVideoSources"
	MediaGetVideoSourceConfiguration         = "GetVideoSourceConfiguration"
	MediaGetVideoSourceConfigurations        = "GetVideoSourceConfigurations"
)

const (
	PTZAbsoluteMove            = "AbsoluteMove"
	PTZContinuousMove          = "ContinuousMove"
	PTZGetConfiguration        = "GetConfiguration"
	PTZGetConfigurations       = "GetConfigurations"
	PTZGetConfigurationOptions = "GetConfigurationOptions"
	PTZGetNode                 = "GetNode"
	PTZGetNodes                = "GetNodes"
	PTZGetPresets              = "GetPresets"
	PTZGetStatus               = "GetStatus"
	PTZGotoHomePosition        = "GotoHomePosition"
	PTZGotoPreset              = "GotoPreset"
	PTZRelativeMove            = "RelativeMove"
	PTZRemovePreset            = "RemovePreset"
	PTZSetHomePosition         = "SetHomePosition"
	PTZSetPreset               = "SetPreset"
	PTZStop                    = "Stop"
)

func GetRequestAction(b []byte) string {
	// <soap-env:Body><ns0:GetCapabilities xmlns:ns0="http://www.onvif.org/ver10/device/wsdl">
	// <v:Body><GetSystemDateAndTime xmlns="http://www.onvif.org/ver10/device/wsdl" /></v:Body>
	re := regexp.MustCompile(`Body[^<]+<([^ />]+)`)
	m := re.FindSubmatch(b)
	if len(m) != 2 {
		return ""
	}
	if i := bytes.IndexByte(m[1], ':'); i > 0 {
		return string(m[1][i+1:])
	}
	return string(m[1])
}

func GetCapabilitiesResponse(host string) []byte {
	return GetCapabilitiesResponseWithQuery(host, "")
}

func GetCapabilitiesResponseWithQuery(host, query string) []byte {
	return GetCapabilitiesResponseWithQueryAndPTZ(host, query, true)
}

func GetCapabilitiesResponseWithQueryAndPTZ(host, query string, ptz bool) []byte {
	e := NewEnvelope()
	e.Appendf(`<tds:GetCapabilitiesResponse>
	<tds:Capabilities>
		<tt:Device>
			<tt:XAddr>http://%s/onvif/device_service%s</tt:XAddr>
		</tt:Device>
		<tt:Media>
			<tt:XAddr>http://%s/onvif/media_service%s</tt:XAddr>
			<tt:StreamingCapabilities>
				<tt:RTPMulticast>false</tt:RTPMulticast>
				<tt:RTP_TCP>false</tt:RTP_TCP>
				<tt:RTP_RTSP_TCP>true</tt:RTP_RTSP_TCP>
			</tt:StreamingCapabilities>
		</tt:Media>
		<tt:Events>
			<tt:XAddr>http://%s/onvif/event%s</tt:XAddr>
			<tt:WSSubscriptionPolicySupport>true</tt:WSSubscriptionPolicySupport>
			<tt:WSPullPointSupport>true</tt:WSPullPointSupport>
			<tt:WSPausableSubscriptionManagerInterfaceSupport>false</tt:WSPausableSubscriptionManagerInterfaceSupport>
		</tt:Events>`, host, query, host, query, host, query)
	if ptz {
		e.Appendf(`<tt:PTZ>
			<tt:XAddr>http://%s/onvif/ptz_service%s</tt:XAddr>
		</tt:PTZ>`, host, query)
	}
	e.Append(`
	</tds:Capabilities>
</tds:GetCapabilitiesResponse>`)
	return e.Bytes()
}

func GetServicesResponse(host string) []byte {
	return GetServicesResponseWithQuery(host, "")
}

func GetServicesResponseWithQuery(host, query string) []byte {
	return GetServicesResponseWithQueryAndPTZ(host, query, true)
}

func GetServicesResponseWithQueryAndPTZ(host, query string, ptz bool) []byte {
	e := NewEnvelope()
	e.Appendf(`<tds:GetServicesResponse>
	<tds:Service>
		<tds:Namespace>http://www.onvif.org/ver10/device/wsdl</tds:Namespace>
		<tds:XAddr>http://%s/onvif/device_service%s</tds:XAddr>
		<tds:Version><tt:Major>2</tt:Major><tt:Minor>5</tt:Minor></tds:Version>
	</tds:Service>
	<tds:Service>
		<tds:Namespace>http://www.onvif.org/ver10/media/wsdl</tds:Namespace>
		<tds:XAddr>http://%s/onvif/media_service%s</tds:XAddr>
		<tds:Version><tt:Major>2</tt:Major><tt:Minor>5</tt:Minor></tds:Version>
	</tds:Service>
	<tds:Service>
		<tds:Namespace>http://www.onvif.org/ver10/events/wsdl</tds:Namespace>
		<tds:XAddr>http://%s/onvif/event%s</tds:XAddr>
		<tds:Version><tt:Major>2</tt:Major><tt:Minor>5</tt:Minor></tds:Version>
	</tds:Service>`, host, query, host, query, host, query)
	if ptz {
		e.Appendf(`<tds:Service>
		<tds:Namespace>http://www.onvif.org/ver20/ptz/wsdl</tds:Namespace>
		<tds:XAddr>http://%s/onvif/ptz_service%s</tds:XAddr>
		<tds:Version><tt:Major>2</tt:Major><tt:Minor>5</tt:Minor></tds:Version>
	</tds:Service>`, host, query)
	}
	e.Append(`
</tds:GetServicesResponse>`)
	return e.Bytes()
}

func GetSystemDateAndTimeResponse() []byte {
	loc := time.Now()
	utc := loc.UTC()

	e := NewEnvelope()
	e.Appendf(`<tds:GetSystemDateAndTimeResponse>
	<tds:SystemDateAndTime>
		<tt:DateTimeType>NTP</tt:DateTimeType>
		<tt:DaylightSavings>true</tt:DaylightSavings>
		<tt:TimeZone>
			<tt:TZ>%s</tt:TZ>
		</tt:TimeZone>
		<tt:UTCDateTime>
			<tt:Time><tt:Hour>%d</tt:Hour><tt:Minute>%d</tt:Minute><tt:Second>%d</tt:Second></tt:Time>
			<tt:Date><tt:Year>%d</tt:Year><tt:Month>%d</tt:Month><tt:Day>%d</tt:Day></tt:Date>
		</tt:UTCDateTime>
		<tt:LocalDateTime>
			<tt:Time><tt:Hour>%d</tt:Hour><tt:Minute>%d</tt:Minute><tt:Second>%d</tt:Second></tt:Time>
			<tt:Date><tt:Year>%d</tt:Year><tt:Month>%d</tt:Month><tt:Day>%d</tt:Day></tt:Date>
		</tt:LocalDateTime>
	</tds:SystemDateAndTime>
</tds:GetSystemDateAndTimeResponse>`,
		GetPosixTZ(loc),
		utc.Hour(), utc.Minute(), utc.Second(), utc.Year(), utc.Month(), utc.Day(),
		loc.Hour(), loc.Minute(), loc.Second(), loc.Year(), loc.Month(), loc.Day(),
	)
	return e.Bytes()
}

func GetDeviceInformationResponse(manuf, model, firmware, serial, hardware string) []byte {
	e := NewEnvelope()
	e.Appendf(`<tds:GetDeviceInformationResponse>
	<tds:Manufacturer>%s</tds:Manufacturer>
	<tds:Model>%s</tds:Model>
	<tds:FirmwareVersion>%s</tds:FirmwareVersion>
	<tds:SerialNumber>%s</tds:SerialNumber>
	<tds:HardwareId>%s</tds:HardwareId>
</tds:GetDeviceInformationResponse>`, escapeServerXML(manuf), escapeServerXML(model), escapeServerXML(firmware), escapeServerXML(serial), escapeServerXML(hardware))
	return e.Bytes()
}

func GetScopesResponse(name, hardware string) []byte {
	e := NewEnvelope()
	e.Appendf(`<tds:GetScopesResponse>
	<tds:Scopes><tt:ScopeDef>Fixed</tt:ScopeDef><tt:ScopeItem>onvif://www.onvif.org/name/%s</tt:ScopeItem></tds:Scopes>
	<tds:Scopes><tt:ScopeDef>Fixed</tt:ScopeDef><tt:ScopeItem>onvif://www.onvif.org/hardware/%s</tt:ScopeItem></tds:Scopes>
	<tds:Scopes><tt:ScopeDef>Fixed</tt:ScopeDef><tt:ScopeItem>onvif://www.onvif.org/location/go2rtc</tt:ScopeItem></tds:Scopes>
	<tds:Scopes><tt:ScopeDef>Fixed</tt:ScopeDef><tt:ScopeItem>onvif://www.onvif.org/Profile/Streaming</tt:ScopeItem></tds:Scopes>
	<tds:Scopes><tt:ScopeDef>Fixed</tt:ScopeDef><tt:ScopeItem>onvif://www.onvif.org/type/Network_Video_Transmitter</tt:ScopeItem></tds:Scopes>
</tds:GetScopesResponse>`, url.PathEscape(name), url.PathEscape(hardware))
	return e.Bytes()
}

type NetworkInterface struct {
	Token        string
	Name         string
	HWAddress    string
	MTU          int
	IPv4         string
	PrefixLength int
}

func GetNetworkInterfacesResponse(interfaces []NetworkInterface) []byte {
	e := NewEnvelope()
	e.Append(`<tds:GetNetworkInterfacesResponse>`)
	for _, iface := range interfaces {
		e.Appendf(`<tds:NetworkInterfaces token="%s"><tt:Enabled>true</tt:Enabled><tt:Info><tt:Name>%s</tt:Name><tt:HwAddress>%s</tt:HwAddress>`,
			escapeServerXML(iface.Token), escapeServerXML(iface.Name), escapeServerXML(iface.HWAddress))
		if iface.MTU > 0 {
			e.Appendf(`<tt:MTU>%d</tt:MTU>`, iface.MTU)
		}
		e.Append(`</tt:Info>`)
		if iface.IPv4 != "" {
			e.Appendf(`<tt:IPv4><tt:Enabled>true</tt:Enabled><tt:Config><tt:Manual><tt:Address>%s</tt:Address><tt:PrefixLength>%d</tt:PrefixLength></tt:Manual><tt:DHCP>false</tt:DHCP></tt:Config></tt:IPv4>`,
				escapeServerXML(iface.IPv4), iface.PrefixLength)
		}
		e.Append(`</tds:NetworkInterfaces>`)
	}
	e.Append(`</tds:GetNetworkInterfacesResponse>`)
	return e.Bytes()
}

func escapeServerXML(value string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(value))
	return buf.String()
}

type Profile struct {
	Name        string
	Token       string
	SourceToken string
	Width       int
	Height      int
	PTZToken    string
	PTZNode     string
}

func GetProfilesResponse(names []string) []byte {
	profiles := make([]Profile, 0, len(names))
	for _, name := range names {
		profiles = append(profiles, Profile{Name: name, Token: name, SourceToken: name})
	}
	return GetProfilesResponseWithProfiles(profiles)
}

func GetProfilesResponseWithProfiles(profiles []Profile) []byte {
	e := NewEnvelope()
	e.Append(`<trt:GetProfilesResponse>`)
	for _, profile := range profiles {
		appendProfile(e, "Profiles", profile)
	}
	e.Append(`</trt:GetProfilesResponse>`)
	return e.Bytes()
}

func GetProfileResponse(name string) []byte {
	return GetProfileResponseWithProfile(Profile{Name: name, Token: name, SourceToken: name})
}

func GetProfileResponseWithProfile(profile Profile) []byte {
	e := NewEnvelope()
	e.Append(`<trt:GetProfileResponse>`)
	appendProfile(e, "Profile", profile)
	e.Append(`</trt:GetProfileResponse>`)
	return e.Bytes()
}

func appendProfile(e *Envelope, tag string, profile Profile) {
	// go2rtc name = ONVIF Profile Name = ONVIF Profile token
	profile = normalizeProfile(profile)
	e.Appendf(`<trt:%s token="%s" fixed="true">`, tag, profile.Token)
	e.Appendf(`<tt:Name>%s</tt:Name>`, profile.Name)
	appendVideoSourceConfiguration(e, "VideoSourceConfiguration", profile.SourceToken)
	appendVideoEncoderConfiguration(e, "VideoEncoderConfiguration", profile)
	if profile.PTZToken != "" {
		appendPTZConfiguration(e, "PTZConfiguration", profile.PTZToken, profile.PTZNode)
	}
	e.Appendf(`</trt:%s>`, tag)
}

func normalizeProfile(profile Profile) Profile {
	if profile.Token == "" {
		profile.Token = profile.Name
	}
	if profile.SourceToken == "" {
		profile.SourceToken = profile.Token
	}
	if profile.Name == "" {
		profile.Name = profile.Token
	}
	if profile.Width <= 0 {
		profile.Width = 1920
	}
	if profile.Height <= 0 {
		profile.Height = 1080
	}
	return profile
}

func GetVideoSourcesResponse(names []string) []byte {
	// go2rtc name = ONVIF VideoSource token
	e := NewEnvelope()
	e.Append(`<trt:GetVideoSourcesResponse>`)
	for _, name := range names {
		e.Appendf(`<trt:VideoSources token="%s">
	<tt:Framerate>30.000000</tt:Framerate>
	<tt:Resolution><tt:Width>1920</tt:Width><tt:Height>1080</tt:Height></tt:Resolution>
</trt:VideoSources>`, name)
	}
	e.Append(`</trt:GetVideoSourcesResponse>`)
	return e.Bytes()
}

func GetVideoSourceConfigurationsResponse(names []string) []byte {
	e := NewEnvelope()
	e.Append(`<trt:GetVideoSourceConfigurationsResponse>`)
	for _, name := range names {
		appendVideoSourceConfiguration(e, "Configurations", name)
	}
	e.Append(`</trt:GetVideoSourceConfigurationsResponse>`)
	return e.Bytes()
}

func GetVideoSourceConfigurationResponse(name string) []byte {
	e := NewEnvelope()
	e.Append(`<trt:GetVideoSourceConfigurationResponse>`)
	appendVideoSourceConfiguration(e, "Configuration", name)
	e.Append(`</trt:GetVideoSourceConfigurationResponse>`)
	return e.Bytes()
}

func appendVideoSourceConfiguration(e *Envelope, tag, name string) {
	// go2rtc name = ONVIF VideoSourceConfiguration token
	e.Appendf(`<tt:%s token="%s" fixed="true">
	<tt:Name>VSC</tt:Name>
	<tt:SourceToken>%s</tt:SourceToken>
	<tt:Bounds x="0" y="0" width="1920" height="1080"></tt:Bounds>
</tt:%s>`, tag, name, name, tag)
}

func GetVideoEncoderConfigurationsResponse() []byte {
	e := NewEnvelope()
	e.Append(`<trt:GetVideoEncoderConfigurationsResponse>`)
	appendVideoEncoderConfiguration(e, "VideoEncoderConfigurations", Profile{})
	e.Append(`</trt:GetVideoEncoderConfigurationsResponse>`)
	return e.Bytes()
}

func GetVideoEncoderConfigurationResponse() []byte {
	e := NewEnvelope()
	e.Append(`<trt:GetVideoEncoderConfigurationResponse>`)
	appendVideoEncoderConfiguration(e, "VideoEncoderConfiguration", Profile{})
	e.Append(`</trt:GetVideoEncoderConfigurationResponse>`)
	return e.Bytes()
}

func appendVideoEncoderConfiguration(e *Envelope, tag string, profile Profile) {
	// empty `RateControl` important for UniFi Protect
	profile = normalizeProfile(profile)
	e.Appendf(`<tt:%s token="vec">
		<tt:Name>VEC</tt:Name>
        <tt:UseCount>1</tt:UseCount>
		<tt:Encoding>H264</tt:Encoding>
		<tt:Resolution><tt:Width>%d</tt:Width><tt:Height>%d</tt:Height></tt:Resolution>
        <tt:Quality>0</tt:Quality>
		<tt:RateControl><tt:FrameRateLimit>30</tt:FrameRateLimit><tt:EncodingInterval>1</tt:EncodingInterval><tt:BitrateLimit>8192</tt:BitrateLimit></tt:RateControl>
        <tt:H264><tt:GovLength>10</tt:GovLength><tt:H264Profile>Main</tt:H264Profile></tt:H264>
        <tt:SessionTimeout>PT10S</tt:SessionTimeout>
	</tt:%s>`, tag, profile.Width, profile.Height, tag)
}

func GetStreamUriResponse(uri string) []byte {
	e := NewEnvelope()
	e.Appendf(`<trt:GetStreamUriResponse><trt:MediaUri><tt:Uri>%s</tt:Uri></trt:MediaUri></trt:GetStreamUriResponse>`, uri)
	return e.Bytes()
}

func GetSnapshotUriResponse(uri string) []byte {
	e := NewEnvelope()
	e.Appendf(`<trt:GetSnapshotUriResponse><trt:MediaUri><tt:Uri>%s</tt:Uri></trt:MediaUri></trt:GetSnapshotUriResponse>`, uri)
	return e.Bytes()
}

func StaticResponse(operation string) []byte {
	switch operation {
	case DeviceGetSystemDateAndTime:
		return GetSystemDateAndTimeResponse()
	case MediaGetVideoEncoderConfiguration:
		return GetVideoEncoderConfigurationResponse()
	case MediaGetVideoEncoderConfigurations:
		return GetVideoEncoderConfigurationsResponse()
	}

	e := NewEnvelope()
	e.Append(responses[operation])
	return e.Bytes()
}

var responses = map[string]string{
	ServiceGetServiceCapabilities: `<trt:GetServiceCapabilitiesResponse>
	<trt:Capabilities SnapshotUri="true" Rotation="false" VideoSourceMode="false" OSD="false" TemporaryOSDText="false" EXICompression="false">
		<trt:StreamingCapabilities RTPMulticast="false" RTP_TCP="false" RTP_RTSP_TCP="true" NonAggregateControl="false" NoRTSPStreaming="false" />
	</trt:Capabilities>
</trt:GetServiceCapabilitiesResponse>`,

	DeviceGetDiscoveryMode:         `<tds:GetDiscoveryModeResponse><tds:DiscoveryMode>Discoverable</tds:DiscoveryMode></tds:GetDiscoveryModeResponse>`,
	DeviceGetDNS:                   `<tds:GetDNSResponse><tds:DNSInformation /></tds:GetDNSResponse>`,
	DeviceGetHostname:              `<tds:GetHostnameResponse><tds:HostnameInformation /></tds:GetHostnameResponse>`,
	DeviceGetNetworkDefaultGateway: `<tds:GetNetworkDefaultGatewayResponse><tds:NetworkGateway /></tds:GetNetworkDefaultGatewayResponse>`,
	DeviceGetNTP:                   `<tds:GetNTPResponse><tds:NTPInformation /></tds:GetNTPResponse>`,
	DeviceSetSystemDateAndTime:     `<tds:SetSystemDateAndTimeResponse />`,
	DeviceSystemReboot:             `<tds:SystemRebootResponse><tds:Message>OK</tds:Message></tds:SystemRebootResponse>`,

	DeviceGetNetworkProtocols:          `<tds:GetNetworkProtocolsResponse />`,
	MediaGetAudioEncoderConfigurations: `<trt:GetAudioEncoderConfigurationsResponse />`,
	MediaGetAudioSources:               `<trt:GetAudioSourcesResponse />`,
	MediaGetAudioSourceConfigurations:  `<trt:GetAudioSourceConfigurationsResponse />`,

	MediaGetVideoEncoderConfigurationOptions: `<trt:GetVideoEncoderConfigurationOptionsResponse>
   <trt:Options>
       <tt:QualityRange><tt:Min>1</tt:Min><tt:Max>6</tt:Max></tt:QualityRange>
	   <tt:H264>
		   <tt:ResolutionsAvailable><tt:Width>1920</tt:Width><tt:Height>1080</tt:Height></tt:ResolutionsAvailable>
		   <tt:GovLengthRange><tt:Min>0</tt:Min><tt:Max>100</tt:Max></tt:GovLengthRange>
		   <tt:FrameRateRange><tt:Min>1</tt:Min><tt:Max>30</tt:Max></tt:FrameRateRange>
		   <tt:EncodingIntervalRange><tt:Min>1</tt:Min><tt:Max>100</tt:Max></tt:EncodingIntervalRange>
           <tt:H264ProfilesSupported>Main</tt:H264ProfilesSupported>
	   </tt:H264>
   </trt:Options>
</trt:GetVideoEncoderConfigurationOptionsResponse>`,
}
