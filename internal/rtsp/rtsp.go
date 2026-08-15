package rtsp

import (
	"errors"
	"io"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/rtsp"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"github.com/AlexxIT/go2rtc/pkg/yaml"
	"github.com/rs/zerolog"
)

type streamQuality struct {
	Width  int `yaml:"width"`
	Height int `yaml:"height"`
}

type authConfig struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

func Init() {
	var conf struct {
		Mod struct {
			Listen       string `yaml:"listen" json:"listen"`
			Username     string `yaml:"username" json:"-"`
			Password     string `yaml:"password" json:"-"`
			DefaultQuery string `yaml:"default_query" json:"default_query"`
			PacketSize   uint16 `yaml:"pkt_size" json:"pkt_size,omitempty"`
		} `yaml:"rtsp"`
	}

	// default config
	conf.Mod.Listen = ":8554"
	conf.Mod.DefaultQuery = "video&audio"

	app.LoadConfig(&conf)
	app.Info["rtsp"] = conf.Mod

	log = app.GetLogger("rtsp")

	// RTSP client support
	streams.HandleFunc("rtsp", rtspHandler)
	streams.HandleFunc("rtsps", rtspHandler)
	streams.HandleFunc("rtspx", rtspHandler)

	// RTSP server support
	address := conf.Mod.Listen
	if address == "" {
		return
	}

	ln, err := net.Listen("tcp", address)
	if err != nil {
		log.Error().Err(err).Msg("[rtsp] listen")
		return
	}

	_, Port, _ = net.SplitHostPort(address)

	log.Info().Str("addr", address).Msg("[rtsp] listen")

	if query, err := url.ParseQuery(conf.Mod.DefaultQuery); err == nil {
		defaultMedias = ParseQuery(query)
	}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}

			c := rtsp.NewServer(conn)
			c.PacketSize = conf.Mod.PacketSize
			if conf.Mod.Username != "" {
				c.Auth(conf.Mod.Username, conf.Mod.Password)
			}
			go tcpHandler(c)
		}
	}()
}

type Handler func(conn *rtsp.Conn) bool

func HandleFunc(handler Handler) {
	handlers = append(handlers, handler)
}

var Port string

func LocalURL(path string) string {
	if path == "" || path[0] != '/' {
		path = "/" + path
	}
	rawURL := "rtsp://127.0.0.1:" + Port + path
	auth := configuredAuth()
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

func configuredAuth() authConfig {
	var cfg struct {
		RTSP authConfig `yaml:"rtsp"`
	}
	app.LoadConfig(&cfg)

	if app.ConfigPath == "" {
		return cfg.RTSP
	}

	data, err := os.ReadFile(app.ConfigPath)
	if err != nil {
		return cfg.RTSP
	}
	if yaml.Unmarshal(data, &cfg) != nil {
		return cfg.RTSP
	}
	return cfg.RTSP
}

// internal

var log zerolog.Logger
var handlers []Handler
var defaultMedias []*core.Media

func rtspHandler(rawURL string) (core.Producer, error) {
	rawURL, rawQuery, _ := strings.Cut(rawURL, "#")

	conn := rtsp.NewClient(rawURL)
	conn.Backchannel = true
	conn.UserAgent = app.UserAgent

	if rawQuery != "" {
		query := streams.ParseQuery(rawQuery)
		conn.Backchannel = query.Get("backchannel") == "1"
		conn.Media = query.Get("media")
		conn.Timeout = core.Atoi(query.Get("timeout"))
		conn.Transport = query.Get("transport")
	}

	if log.Trace().Enabled() {
		conn.Listen(func(msg any) {
			switch msg := msg.(type) {
			case *tcp.Request:
				log.Trace().Msgf("[rtsp] client request:\n%s", msg)
			case *tcp.Response:
				log.Trace().Msgf("[rtsp] client response:\n%s", msg)
			case string:
				log.Trace().Msgf("[rtsp] client msg: %s", msg)
			}
		})
	}

	if err := conn.Dial(); err != nil {
		return nil, err
	}

	if err := conn.Describe(); err != nil {
		if !conn.Backchannel {
			return nil, err
		}
		log.Trace().Msgf("[rtsp] describe (backchannel=%t) err: %v", conn.Backchannel, err)

		// second try without backchannel, we need to reconnect
		conn.Backchannel = false
		if err = conn.Dial(); err != nil {
			return nil, err
		}
		if err = conn.Describe(); err != nil {
			return nil, err
		}
	}

	return conn, nil
}

func tcpHandler(conn *rtsp.Conn) {
	var name string
	var closer func()

	trace := log.Trace().Enabled()
	level := zerolog.WarnLevel

	conn.Listen(func(msg any) {
		if trace {
			switch msg := msg.(type) {
			case *tcp.Request:
				log.Trace().Msgf("[rtsp] server request:\n%s", msg)
			case *tcp.Response:
				log.Trace().Msgf("[rtsp] server response:\n%s", msg)
			}
		}

		switch msg {
		case rtsp.MethodDescribe:
			if len(conn.URL.Path) == 0 {
				log.Warn().Msg("[rtsp] server empty URL on DESCRIBE")
				return
			}

			name = conn.URL.Path[1:]
			query := conn.URL.Query()

			stream := streams.Get(name)
			if stream == nil {
				if baseName, quality, ok := resolveRTSPQualityAlias(name, streams.GetAllNames()); ok {
					name = baseName
					if quality.Width > 0 {
						query.Set("onvif_width", strconv.Itoa(quality.Width))
					}
					if quality.Height > 0 {
						query.Set("onvif_height", strconv.Itoa(quality.Height))
					}
					stream = streams.Get(name)
				}
			}
			if stream == nil {
				return
			}

			log.Debug().Str("stream", name).Msg("[rtsp] new consumer")

			conn.SessionName = app.UserAgent

			if stream = onvifQualityStream(name, query, stream); stream == nil {
				return
			}
			query.Del("onvif_width")
			query.Del("onvif_height")
			conn.Medias = ParseQuery(query)
			if conn.Medias == nil {
				for _, media := range defaultMedias {
					conn.Medias = append(conn.Medias, media.Clone())
				}
			}

			if query.Get("backchannel") == "1" {
				conn.Medias = append(conn.Medias, &core.Media{
					Kind:      core.KindAudio,
					Direction: core.DirectionRecvonly,
					Codecs: []*core.Codec{
						{Name: core.CodecOpus, ClockRate: 48000, Channels: 2},
						{Name: core.CodecPCM, ClockRate: 16000},
						{Name: core.CodecPCMA, ClockRate: 16000},
						{Name: core.CodecPCMU, ClockRate: 16000},
						{Name: core.CodecPCM, ClockRate: 8000},
						{Name: core.CodecPCMA, ClockRate: 8000},
						{Name: core.CodecPCMU, ClockRate: 8000},
						{Name: core.CodecAAC, ClockRate: 8000},
						{Name: core.CodecAAC, ClockRate: 16000},
					},
				})
			}

			if s := query.Get("pkt_size"); s != "" {
				conn.PacketSize = uint16(core.Atoi(s))
			}

			// param name like ffmpeg style https://ffmpeg.org/ffmpeg-protocols.html
			if s := query.Get("log_level"); s != "" {
				if lvl, err := zerolog.ParseLevel(s); err == nil {
					level = lvl
				}
			}

			// will help to protect looping requests to same source
			conn.Connection.Source = query.Get("source")

			if err := stream.AddConsumer(conn); err != nil {
				log.WithLevel(level).Err(err).Str("stream", name).Msg("[rtsp]")
				return
			}

			closer = func() {
				stream.RemoveConsumer(conn)
			}

		case rtsp.MethodAnnounce:
			if len(conn.URL.Path) == 0 {
				log.Warn().Msg("[rtsp] server empty URL on ANNOUNCE")
				return
			}

			name = conn.URL.Path[1:]

			stream := streams.Get(name)
			if stream == nil {
				return
			}

			query := conn.URL.Query()
			if s := query.Get("timeout"); s != "" {
				conn.Timeout = core.Atoi(s)
			}

			log.Debug().Str("stream", name).Msg("[rtsp] new producer")

			stream.AddProducer(conn)

			closer = func() {
				stream.RemoveProducer(conn)
			}
		}
	})

	if err := conn.Accept(); err != nil {
		if errors.Is(err, rtsp.FailedAuth) {
			log.Warn().Str("remote_addr", conn.Connection.RemoteAddr).Msg("[rtsp] failed authentication")
		} else if err != io.EOF {
			log.WithLevel(level).Err(err).Caller().Send()
		}
		if closer != nil {
			closer()
		}
		_ = conn.Close()
		return
	}

	for _, handler := range handlers {
		if handler(conn) {
			return
		}
	}

	if closer != nil {
		if err := conn.Handle(); err != nil {
			log.Debug().Err(err).Msg("[rtsp] handle")
		}

		closer()

		log.Debug().Str("stream", name).Msg("[rtsp] disconnect")
	}

	_ = conn.Close()
}

func onvifQualityStream(name string, query url.Values, fallback *streams.Stream) *streams.Stream {
	width := core.Atoi(query.Get("onvif_width"))
	height := core.Atoi(query.Get("onvif_height"))
	if query.Get("onvif_ptz") == "1" {
		return onvifPTZStreamFor(name, streamQuality{Width: width, Height: height})
	}
	if width <= 0 && height <= 0 {
		return fallback
	}
	quality := normalizeRTSPQuality(streamQuality{Width: width, Height: height})

	params := []string{"video=h264", "audio=copy"}
	if quality.Width > 0 {
		params = append(params, "width="+strconv.Itoa(quality.Width))
	}
	if quality.Height > 0 {
		params = append(params, "height="+strconv.Itoa(quality.Height))
	}
	stream := streams.NewStream([]string{"ffmpeg:" + name + "#" + strings.Join(params, "#")})
	return stream
}

func resolveRTSPQualityAlias(alias string, names []string) (string, streamQuality, bool) {
	for _, name := range names {
		prefix := name + "_"
		if !strings.HasPrefix(alias, prefix) {
			continue
		}
		token := alias[len(prefix):]
		for _, quality := range configuredRTSPStreamQualities(name) {
			if quality.Width <= 0 && quality.Height <= 0 {
				continue
			}
			if rtspQualityToken(quality) == token {
				return name, normalizeRTSPQuality(quality), true
			}
		}
	}
	return "", streamQuality{}, false
}

func configuredRTSPStreamQualities(name string) []streamQuality {
	if app.ConfigPath == "" || name == "" {
		return nil
	}
	data, err := os.ReadFile(app.ConfigPath)
	if err != nil {
		return nil
	}
	var cfg struct {
		Simulate struct {
			ONVIFQuality   map[string]streamQuality   `yaml:"onvif_quality"`
			ONVIFQualities map[string][]streamQuality `yaml:"onvif_qualities"`
		} `yaml:"simulate"`
	}
	if yaml.Unmarshal(data, &cfg) != nil {
		return nil
	}
	if qualities, ok := cfg.Simulate.ONVIFQualities[name]; ok {
		return qualities
	}
	if quality, ok := cfg.Simulate.ONVIFQuality[name]; ok {
		return []streamQuality{quality}
	}
	return nil
}

func normalizeRTSPQuality(quality streamQuality) streamQuality {
	if quality.Width > 0 || quality.Height <= 0 {
		return quality
	}
	width := (quality.Height*16 + 4) / 9
	if width%2 != 0 {
		width++
	}
	quality.Width = width
	return quality
}

func rtspQualityToken(quality streamQuality) string {
	if quality.Width > 0 && quality.Height > 0 {
		return strconv.Itoa(quality.Width) + "x" + strconv.Itoa(quality.Height)
	}
	if quality.Height > 0 {
		return strconv.Itoa(quality.Height) + "p"
	}
	if quality.Width > 0 {
		return strconv.Itoa(quality.Width) + "w"
	}
	return "original"
}

func ParseQuery(query map[string][]string) []*core.Media {
	if v := query["mp4"]; v != nil {
		return []*core.Media{
			{
				Kind:      core.KindVideo,
				Direction: core.DirectionSendonly,
				Codecs: []*core.Codec{
					{Name: core.CodecH264},
					{Name: core.CodecH265},
				},
			},
			{
				Kind:      core.KindAudio,
				Direction: core.DirectionSendonly,
				Codecs: []*core.Codec{
					{Name: core.CodecAAC},
				},
			},
		}
	}

	return core.ParseQuery(query)
}
