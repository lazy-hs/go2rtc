package onvif

import (
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/rtsp"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/onvif"
	"github.com/AlexxIT/go2rtc/pkg/yaml"
	"github.com/rs/zerolog"
)

type onvifStreamQuality struct {
	Width  int `yaml:"width"`
	Height int `yaml:"height"`
}

type rtspAuthConfig struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

func Init() {
	log = app.GetLogger("onvif")

	var cfg struct {
		Event  eventConfig  `yaml:"event"`
		Device deviceConfig `yaml:"onvif"`
	}
	app.LoadConfig(&cfg)
	device = cfg.Device.withDefaults(app.Version)
	events = newEventManager(cfg.Event)
	events.start()
	if len(events.templates) == 0 {
		log.Warn().Msg("[onvif] event generator disabled: event.templates is empty")
	} else if !events.enabled {
		log.Info().Int("templates", len(events.templates)).Msg("[onvif] event generator disabled by config")
	} else {
		log.Info().Dur("interval", events.interval).Int("burst", events.burst).Bool("permanent", events.permanent).
			Int("templates", len(events.templates)).Msg("[onvif] event generator enabled")
	}

	streams.HandleFunc("onvif", streamOnvif)

	// ONVIF server on all suburls
	api.HandleFunc("/onvif/", onvifDeviceService)
	api.HandleFunc("api/simulate/events", apiSimulateEvents)
	startDiscovery(api.Port)

	// ONVIF client autodiscovery
	api.HandleFunc("api/onvif", apiOnvif)
}

var log zerolog.Logger

func streamOnvif(rawURL string) (core.Producer, error) {
	client, err := onvif.NewClient(rawURL)
	if err != nil {
		return nil, err
	}

	uri, err := client.GetURI()
	if err != nil {
		return nil, err
	}

	// Append hash-based arguments to the retrieved URI
	if i := strings.IndexByte(rawURL, '#'); i > 0 {
		uri += rawURL[i:]
	}

	log.Debug().Msgf("[onvif] new uri=%s", uri)

	if err = streams.Validate(uri); err != nil {
		return nil, err
	}

	return streams.GetProducer(uri)
}

func onvifDeviceService(w http.ResponseWriter, r *http.Request) {
	b, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	request := b
	operation := onvif.GetRequestAction(request)
	if operation == "" {
		http.Error(w, "malformed request body", http.StatusBadRequest)
		return
	}

	log.Trace().Msgf("[onvif] server request %s %s:\n%s", r.Method, r.RequestURI, b)

	auth := configuredRTSPAuth()
	if onvifAuthRequired(operation, auth) && !validateONVIFAuth(r, request, auth) {
		writeONVIFAuthError(w)
		log.Warn().Str("remote_addr", r.RemoteAddr).Str("operation", operation).Msg("[onvif] failed authentication")
		return
	}

	if isEventRequest(r, operation) {
		b, err = eventResponse(r, request, operation)
		if err != nil {
			status := http.StatusBadRequest
			if err == errSubscriptionNotFound {
				status = http.StatusNotFound
			}
			http.Error(w, err.Error(), status)
			log.Warn().Err(err).Str("operation", operation).Msg("[onvif] event request")
			return
		}
		if b != nil {
			action := eventResponseAction(operation)
			b = eventAddressedResponse(r, request, b, action)
			writeSOAPResponseAction(w, b, action)
			return
		}
	}

	switch operation {
	case onvif.ServiceGetServiceCapabilities, // important for Hass
		onvif.DeviceGetSystemDateAndTime, // important for Hass
		onvif.DeviceSetSystemDateAndTime, // return just OK
		onvif.DeviceGetDiscoveryMode,
		onvif.DeviceGetDNS,
		onvif.DeviceGetHostname,
		onvif.DeviceGetNetworkDefaultGateway,
		onvif.DeviceGetNetworkProtocols,
		onvif.DeviceGetNTP,
		onvif.MediaGetVideoEncoderConfiguration,
		onvif.MediaGetVideoEncoderConfigurations,
		onvif.MediaGetAudioEncoderConfigurations,
		onvif.MediaGetVideoEncoderConfigurationOptions,
		onvif.MediaGetAudioSources,
		onvif.MediaGetAudioSourceConfigurations:
		b = onvif.StaticResponse(operation)

	case onvif.DeviceGetCapabilities:
		// important for Hass: Media section
		b = onvif.GetCapabilitiesResponseWithQuery(r.Host, onvifServiceQuery(r))

	case onvif.DeviceGetServices:
		b = onvif.GetServicesResponseWithQuery(r.Host, onvifServiceQuery(r))

	case onvif.DeviceGetDeviceInformation:
		// important for Hass: SerialNumber (unique server ID)
		serial := device.Serial
		if serial == "" {
			serial = r.Host
		}
		b = onvif.GetDeviceInformationResponse(
			device.Manufacturer, device.Model, device.Firmware, serial, device.Hardware,
		)

	case onvif.DeviceGetScopes:
		b = onvif.GetScopesResponse(device.Name, device.Hardware)

	case onvif.DeviceGetNetworkInterfaces:
		interfaces := networkInterfacesForRequest(r)
		b = onvif.GetNetworkInterfacesResponse(interfaces)
		logNetworkInterface(interfaces)

	case onvif.DeviceSystemReboot:
		b = onvif.StaticResponse(operation)

		time.AfterFunc(time.Second, func() {
			os.Exit(0)
		})

	case onvif.MediaGetVideoSources:
		b = onvif.GetVideoSourcesResponse(streams.GetAllNames())

	case onvif.MediaGetProfiles:
		// important for Hass: H264 codec, width, height
		b = onvif.GetProfilesResponseWithProfiles(configuredONVIFProfiles(streams.GetAllNames()))

	case onvif.MediaGetProfile:
		token := onvif.FindTagValue(b, "ProfileToken")
		if profile, ok := configuredONVIFProfile(token); ok {
			b = onvif.GetProfileResponseWithProfile(profile)
		} else {
			b = onvif.GetProfileResponse(token)
		}

	case onvif.MediaGetVideoSourceConfigurations:
		// important for Happytime Onvif Client
		b = onvif.GetVideoSourceConfigurationsResponse(streams.GetAllNames())

	case onvif.MediaGetVideoSourceConfiguration:
		token := onvif.FindTagValue(b, "ConfigurationToken")
		b = onvif.GetVideoSourceConfigurationResponse(token)

	case onvif.MediaGetStreamUri:
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host // in case of Host without port
		}

		token := onvif.FindTagValue(b, "ProfileToken")
		profile, ok := configuredONVIFProfile(token)
		if !ok {
			profile = onvif.Profile{Name: token, Token: token, SourceToken: token}
		}
		uri := "rtsp://" + host + ":" + rtsp.Port + "/" + rtspPathForONVIFProfile(profile)
		uri = applyRTSPAuth(uri, configuredRTSPAuth())
		b = onvif.GetStreamUriResponse(uri)

	case onvif.MediaGetSnapshotUri:
		token := onvif.FindTagValue(b, "ProfileToken")
		if profile, ok := configuredONVIFProfile(token); ok {
			token = profile.SourceToken
		}
		uri := "http://" + r.Host + "/api/frame.jpeg?src=" + token
		b = onvif.GetSnapshotUriResponse(uri)

	default:
		http.Error(w, "unsupported operation", http.StatusBadRequest)
		log.Warn().Msgf("[onvif] unsupported operation: %s", operation)
		log.Debug().Msgf("[onvif] unsupported request:\n%s", b)
		return
	}

	log.Trace().Msgf("[onvif] server response:\n%s", b)

	writeSOAPResponse(w, b)
}

func configuredONVIFProfiles(names []string) []onvif.Profile {
	profiles := make([]onvif.Profile, 0, len(names))
	for _, name := range names {
		qualities := configuredONVIFStreamQualities(name)
		if len(qualities) == 0 {
			qualities = []onvifStreamQuality{{}}
		}
		seen := map[string]bool{}
		for _, quality := range qualities {
			profile := onvifProfileForQuality(name, quality)
			if seen[profile.Token] {
				continue
			}
			seen[profile.Token] = true
			profiles = append(profiles, profile)
		}
	}
	return profiles
}

func configuredONVIFProfile(token string) (onvif.Profile, bool) {
	for _, profile := range configuredONVIFProfiles(streams.GetAllNames()) {
		if profile.Token == token {
			return profile, true
		}
	}
	return onvif.Profile{}, false
}

func configuredONVIFStreamQualities(name string) []onvifStreamQuality {
	if app.ConfigPath == "" || name == "" {
		return nil
	}
	data, err := os.ReadFile(app.ConfigPath)
	if err != nil {
		return nil
	}
	var cfg struct {
		Simulate struct {
			ONVIFQuality   map[string]onvifStreamQuality   `yaml:"onvif_quality"`
			ONVIFQualities map[string][]onvifStreamQuality `yaml:"onvif_qualities"`
		} `yaml:"simulate"`
	}
	if yaml.Unmarshal(data, &cfg) != nil {
		return nil
	}
	if qualities, ok := cfg.Simulate.ONVIFQualities[name]; ok {
		return qualities
	}
	if quality, ok := cfg.Simulate.ONVIFQuality[name]; ok {
		return []onvifStreamQuality{quality}
	}
	return nil
}

func onvifProfileForQuality(name string, quality onvifStreamQuality) onvif.Profile {
	if quality.Width <= 0 && quality.Height <= 0 {
		return onvif.Profile{Name: name, Token: name, SourceToken: name}
	}
	label := onvifQualityLabel(quality)
	return onvif.Profile{
		Name:        name + " " + label,
		Token:       name + "__onvif_" + onvifQualityToken(quality),
		SourceToken: name,
		Width:       onvifQualityProfileWidth(quality),
		Height:      quality.Height,
	}
}

func rtspPathForONVIFProfile(profile onvif.Profile) string {
	if profile.Width <= 0 && profile.Height <= 0 {
		return profile.SourceToken
	}
	prefix := profile.SourceToken + "__onvif_"
	if token, ok := strings.CutPrefix(profile.Token, prefix); ok && token != "" {
		return profile.SourceToken + "_" + token
	}
	return profile.SourceToken + "_" + onvifQualityToken(onvifStreamQuality{Width: profile.Width, Height: profile.Height})
}

func onvifQualityProfileWidth(quality onvifStreamQuality) int {
	if quality.Width > 0 {
		return quality.Width
	}
	if quality.Height <= 0 {
		return 0
	}
	width := (quality.Height*16 + 4) / 9
	if width%2 != 0 {
		width++
	}
	return width
}

func onvifQualityLabel(quality onvifStreamQuality) string {
	if quality.Width > 0 && quality.Height > 0 {
		return strconv.Itoa(quality.Width) + "x" + strconv.Itoa(quality.Height)
	}
	if quality.Height > 0 {
		return strconv.Itoa(quality.Height) + "p"
	}
	if quality.Width > 0 {
		return strconv.Itoa(quality.Width) + "w"
	}
	return "原始"
}

func onvifQualityToken(quality onvifStreamQuality) string {
	label := strings.ToLower(onvifQualityLabel(quality))
	label = strings.NewReplacer(" ", "_", "×", "x").Replace(label)
	var b strings.Builder
	for _, r := range label {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "original"
	}
	return b.String()
}

func configuredRTSPAuth() rtspAuthConfig {
	if app.ConfigPath == "" {
		return rtspAuthConfig{}
	}
	data, err := os.ReadFile(app.ConfigPath)
	if err != nil {
		return rtspAuthConfig{}
	}
	var cfg struct {
		RTSP rtspAuthConfig `yaml:"rtsp"`
	}
	if yaml.Unmarshal(data, &cfg) != nil {
		return rtspAuthConfig{}
	}
	return cfg.RTSP
}

func applyRTSPAuth(rawURL string, auth rtspAuthConfig) string {
	if strings.TrimSpace(auth.Username) == "" {
		return rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	username := strings.TrimSpace(auth.Username)
	password := strings.TrimSpace(auth.Password)
	if password == "" {
		u.User = url.User(username)
	} else {
		u.User = url.UserPassword(username, password)
	}
	return u.String()
}

func applyONVIFStreamQuality(rawURL string, quality onvifStreamQuality) string {
	if quality.Width <= 0 && quality.Height <= 0 {
		return rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	query := u.Query()
	if quality.Width > 0 {
		query.Set("onvif_width", strconv.Itoa(quality.Width))
	}
	if quality.Height > 0 {
		query.Set("onvif_height", strconv.Itoa(quality.Height))
	}
	u.RawQuery = query.Encode()
	return u.String()
}

func writeSOAPResponse(w http.ResponseWriter, b []byte) {
	w.Header().Set("Content-Type", "application/soap+xml; charset=utf-8")
	if _, err := w.Write(b); err != nil {
		log.Error().Err(err).Caller().Send()
	}
}

func writeSOAPResponseAction(w http.ResponseWriter, b []byte, action string) {
	if action == "" {
		writeSOAPResponse(w, b)
		return
	}
	w.Header().Set("Content-Type", `application/soap+xml; charset=utf-8; action="`+action+`"`)
	w.Header().Set("SOAPAction", `"`+action+`"`)
	if _, err := w.Write(b); err != nil {
		log.Error().Err(err).Caller().Send()
	}
}

func apiOnvif(w http.ResponseWriter, r *http.Request) {
	src := r.URL.Query().Get("src")

	var items []*api.Source

	if src == "" {
		devices, err := onvif.DiscoveryStreamingDevices()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		for _, device := range devices {
			u, err := url.Parse(device.URL)
			if err != nil {
				log.Warn().Str("url", device.URL).Msg("[onvif] broken")
				continue
			}

			if u.Scheme != "http" {
				log.Warn().Str("url", device.URL).Msg("[onvif] unsupported")
				continue
			}

			u.Scheme = "onvif"
			u.User = url.UserPassword("user", "pass")

			if u.Path == onvif.PathDevice {
				u.Path = ""
			}

			items = append(items, &api.Source{
				Name: u.Host,
				URL:  u.String(),
				Info: device.Name + " " + device.Hardware,
			})
		}
	} else {
		client, err := onvif.NewClient(src)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if l := log.Trace(); l.Enabled() {
			b, _ := client.MediaRequest(onvif.MediaGetProfiles)
			l.Msgf("[onvif] src=%s profiles:\n%s", src, b)
		}

		name, err := client.GetName()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		tokens, err := client.GetProfilesTokens()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		for i, token := range tokens {
			items = append(items, &api.Source{
				Name: name + " stream" + strconv.Itoa(i),
				URL:  src + "?subtype=" + token,
			})
		}

		if len(tokens) > 0 && client.HasSnapshots() {
			items = append(items, &api.Source{
				Name: name + " snapshot",
				URL:  src + "?subtype=" + tokens[0] + "&snapshot",
			})
		}
	}

	api.ResponseSources(w, items)
}
