package onvif

import "time"

const (
	PTZPanTiltPositionSpace  = "http://www.onvif.org/ver10/tptz/PanTiltSpaces/PositionGenericSpace"
	PTZZoomPositionSpace     = "http://www.onvif.org/ver10/tptz/ZoomSpaces/PositionGenericSpace"
	PTZPanTiltVelocitySpace  = "http://www.onvif.org/ver10/tptz/PanTiltSpaces/VelocityGenericSpace"
	PTZZoomVelocitySpace     = "http://www.onvif.org/ver10/tptz/ZoomSpaces/VelocityGenericSpace"
	PTZPanTiltTranslateSpace = "http://www.onvif.org/ver10/tptz/PanTiltSpaces/TranslationGenericSpace"
	PTZZoomTranslateSpace    = "http://www.onvif.org/ver10/tptz/ZoomSpaces/TranslationGenericSpace"
)

type PTZPosition struct {
	Pan  float64
	Tilt float64
	Zoom float64
}

type PTZNode struct {
	Token string
	Name  string
}

type PTZConfiguration struct {
	Token     string
	Name      string
	NodeToken string
}

type PTZPreset struct {
	Token    string
	Name     string
	Position PTZPosition
}

func appendPTZConfiguration(e *Envelope, tag, token, node string) {
	if node == "" {
		node = token
	}
	e.Appendf(`<tt:%s token="%s"><tt:Name>PTZ</tt:Name><tt:UseCount>1</tt:UseCount><tt:NodeToken>%s</tt:NodeToken><tt:DefaultAbsolutePantTiltPositionSpace>%s</tt:DefaultAbsolutePantTiltPositionSpace><tt:DefaultAbsoluteZoomPositionSpace>%s</tt:DefaultAbsoluteZoomPositionSpace><tt:DefaultRelativePanTiltTranslationSpace>%s</tt:DefaultRelativePanTiltTranslationSpace><tt:DefaultRelativeZoomTranslationSpace>%s</tt:DefaultRelativeZoomTranslationSpace><tt:DefaultContinuousPanTiltVelocitySpace>%s</tt:DefaultContinuousPanTiltVelocitySpace><tt:DefaultContinuousZoomVelocitySpace>%s</tt:DefaultContinuousZoomVelocitySpace><tt:PanTiltLimits><tt:Range><tt:URI>%s</tt:URI><tt:XRange><tt:Min>-1</tt:Min><tt:Max>1</tt:Max></tt:XRange><tt:YRange><tt:Min>-1</tt:Min><tt:Max>1</tt:Max></tt:YRange></tt:Range></tt:PanTiltLimits><tt:ZoomLimits><tt:Range><tt:URI>%s</tt:URI><tt:XRange><tt:Min>0</tt:Min><tt:Max>1</tt:Max></tt:XRange></tt:Range></tt:ZoomLimits></tt:%s>`,
		tag, escapeServerXML(token), escapeServerXML(node), PTZPanTiltPositionSpace, PTZZoomPositionSpace,
		PTZPanTiltTranslateSpace, PTZZoomTranslateSpace, PTZPanTiltVelocitySpace, PTZZoomVelocitySpace,
		PTZPanTiltPositionSpace, PTZZoomPositionSpace, tag)
}

func GetPTZServiceCapabilitiesResponse() []byte {
	e := NewEnvelope()
	e.Append(`<tptz:GetServiceCapabilitiesResponse><tptz:Capabilities EFlip="false" Reverse="false" GetCompatibleConfigurations="true" MoveStatus="true" StatusPosition="true" /></tptz:GetServiceCapabilitiesResponse>`)
	return e.Bytes()
}

func GetPTZNodesResponse(nodes []PTZNode) []byte {
	e := NewEnvelope()
	e.Append(`<tptz:GetNodesResponse>`)
	for _, node := range nodes {
		appendPTZNode(e, "PTZNode", node)
	}
	e.Append(`</tptz:GetNodesResponse>`)
	return e.Bytes()
}

func GetPTZNodeResponse(node PTZNode) []byte {
	e := NewEnvelope()
	e.Append(`<tptz:GetNodeResponse>`)
	appendPTZNode(e, "PTZNode", node)
	e.Append(`</tptz:GetNodeResponse>`)
	return e.Bytes()
}

func appendPTZNode(e *Envelope, tag string, node PTZNode) {
	e.Appendf(`<tptz:%s token="%s"><tt:Name>%s</tt:Name><tt:SupportedPTZSpaces><tt:AbsolutePanTiltPositionSpace><tt:URI>%s</tt:URI><tt:XRange><tt:Min>-1</tt:Min><tt:Max>1</tt:Max></tt:XRange><tt:YRange><tt:Min>-1</tt:Min><tt:Max>1</tt:Max></tt:YRange></tt:AbsolutePanTiltPositionSpace><tt:AbsoluteZoomPositionSpace><tt:URI>%s</tt:URI><tt:XRange><tt:Min>0</tt:Min><tt:Max>1</tt:Max></tt:XRange></tt:AbsoluteZoomPositionSpace><tt:RelativePanTiltTranslationSpace><tt:URI>%s</tt:URI><tt:XRange><tt:Min>-1</tt:Min><tt:Max>1</tt:Max></tt:XRange><tt:YRange><tt:Min>-1</tt:Min><tt:Max>1</tt:Max></tt:YRange></tt:RelativePanTiltTranslationSpace><tt:RelativeZoomTranslationSpace><tt:URI>%s</tt:URI><tt:XRange><tt:Min>-1</tt:Min><tt:Max>1</tt:Max></tt:XRange></tt:RelativeZoomTranslationSpace><tt:ContinuousPanTiltVelocitySpace><tt:URI>%s</tt:URI><tt:XRange><tt:Min>-1</tt:Min><tt:Max>1</tt:Max></tt:XRange><tt:YRange><tt:Min>-1</tt:Min><tt:Max>1</tt:Max></tt:YRange></tt:ContinuousPanTiltVelocitySpace><tt:ContinuousZoomVelocitySpace><tt:URI>%s</tt:URI><tt:XRange><tt:Min>-1</tt:Min><tt:Max>1</tt:Max></tt:XRange></tt:ContinuousZoomVelocitySpace></tt:SupportedPTZSpaces><tt:MaximumNumberOfPresets>32</tt:MaximumNumberOfPresets><tt:HomeSupported>true</tt:HomeSupported></tptz:%s>`,
		tag, escapeServerXML(node.Token), escapeServerXML(node.Name), PTZPanTiltPositionSpace, PTZZoomPositionSpace,
		PTZPanTiltTranslateSpace, PTZZoomTranslateSpace, PTZPanTiltVelocitySpace, PTZZoomVelocitySpace, tag)
}

func GetPTZConfigurationsResponse(configs []PTZConfiguration) []byte {
	e := NewEnvelope()
	e.Append(`<tptz:GetConfigurationsResponse>`)
	for _, config := range configs {
		appendPTZConfiguration(e, "PTZConfiguration", config.Token, config.NodeToken)
	}
	e.Append(`</tptz:GetConfigurationsResponse>`)
	return e.Bytes()
}

func GetPTZConfigurationResponse(config PTZConfiguration) []byte {
	e := NewEnvelope()
	e.Append(`<tptz:GetConfigurationResponse>`)
	appendPTZConfiguration(e, "PTZConfiguration", config.Token, config.NodeToken)
	e.Append(`</tptz:GetConfigurationResponse>`)
	return e.Bytes()
}

func GetPTZConfigurationOptionsResponse() []byte {
	e := NewEnvelope()
	e.Appendf(`<tptz:GetConfigurationOptionsResponse><tptz:PTZConfigurationOptions><tt:Spaces><tt:AbsolutePanTiltPositionSpace><tt:URI>%s</tt:URI><tt:XRange><tt:Min>-1</tt:Min><tt:Max>1</tt:Max></tt:XRange><tt:YRange><tt:Min>-1</tt:Min><tt:Max>1</tt:Max></tt:YRange></tt:AbsolutePanTiltPositionSpace><tt:AbsoluteZoomPositionSpace><tt:URI>%s</tt:URI><tt:XRange><tt:Min>0</tt:Min><tt:Max>1</tt:Max></tt:XRange></tt:AbsoluteZoomPositionSpace><tt:RelativePanTiltTranslationSpace><tt:URI>%s</tt:URI><tt:XRange><tt:Min>-1</tt:Min><tt:Max>1</tt:Max></tt:XRange><tt:YRange><tt:Min>-1</tt:Min><tt:Max>1</tt:Max></tt:YRange></tt:RelativePanTiltTranslationSpace><tt:RelativeZoomTranslationSpace><tt:URI>%s</tt:URI><tt:XRange><tt:Min>-1</tt:Min><tt:Max>1</tt:Max></tt:XRange></tt:RelativeZoomTranslationSpace><tt:ContinuousPanTiltVelocitySpace><tt:URI>%s</tt:URI><tt:XRange><tt:Min>-1</tt:Min><tt:Max>1</tt:Max></tt:XRange><tt:YRange><tt:Min>-1</tt:Min><tt:Max>1</tt:Max></tt:YRange></tt:ContinuousPanTiltVelocitySpace><tt:ContinuousZoomVelocitySpace><tt:URI>%s</tt:URI><tt:XRange><tt:Min>-1</tt:Min><tt:Max>1</tt:Max></tt:XRange></tt:ContinuousZoomVelocitySpace></tt:Spaces><tt:PTZTimeout><tt:Min>PT0S</tt:Min><tt:Max>PT1H</tt:Max></tt:PTZTimeout></tptz:PTZConfigurationOptions></tptz:GetConfigurationOptionsResponse>`,
		PTZPanTiltPositionSpace, PTZZoomPositionSpace, PTZPanTiltTranslateSpace, PTZZoomTranslateSpace,
		PTZPanTiltVelocitySpace, PTZZoomVelocitySpace)
	return e.Bytes()
}

func GetPTZStatusResponse(position PTZPosition, panTiltMoving, zoomMoving bool) []byte {
	panTiltStatus := "IDLE"
	if panTiltMoving {
		panTiltStatus = "MOVING"
	}
	zoomStatus := "IDLE"
	if zoomMoving {
		zoomStatus = "MOVING"
	}
	e := NewEnvelope()
	e.Appendf(`<tptz:GetStatusResponse><tptz:PTZStatus><tt:Position><tt:PanTilt x="%.6f" y="%.6f" space="%s"/><tt:Zoom x="%.6f" space="%s"/></tt:Position><tt:MoveStatus><tt:PanTilt>%s</tt:PanTilt><tt:Zoom>%s</tt:Zoom></tt:MoveStatus><tt:UtcTime>%s</tt:UtcTime></tptz:PTZStatus></tptz:GetStatusResponse>`,
		position.Pan, position.Tilt, PTZPanTiltPositionSpace, position.Zoom, PTZZoomPositionSpace,
		panTiltStatus, zoomStatus, timeNowUTC())
	return e.Bytes()
}

func GetPTZMoveResponse(operation string) []byte {
	e := NewEnvelope()
	e.Appendf(`<tptz:%sResponse/>`, operation)
	return e.Bytes()
}

func GetPTZPresetsResponse(presets []PTZPreset) []byte {
	e := NewEnvelope()
	e.Append(`<tptz:GetPresetsResponse>`)
	for _, preset := range presets {
		e.Appendf(`<tptz:Preset token="%s"><tt:Name>%s</tt:Name><tt:PTZPosition><tt:PanTilt x="%.6f" y="%.6f" space="%s"/><tt:Zoom x="%.6f" space="%s"/></tt:PTZPosition></tptz:Preset>`,
			escapeServerXML(preset.Token), escapeServerXML(preset.Name), preset.Position.Pan, preset.Position.Tilt,
			PTZPanTiltPositionSpace, preset.Position.Zoom, PTZZoomPositionSpace)
	}
	e.Append(`</tptz:GetPresetsResponse>`)
	return e.Bytes()
}

func SetPTZPresetResponse(token string) []byte {
	e := NewEnvelope()
	e.Appendf(`<tptz:SetPresetResponse><tptz:PresetToken>%s</tptz:PresetToken></tptz:SetPresetResponse>`, escapeServerXML(token))
	return e.Bytes()
}

var timeNowUTC = func() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
