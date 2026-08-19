package ffmpeg

import (
	"strings"
	"sync"

	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
)

type producerGetter func(string) (core.Producer, error)

var rtspAudioFallbacks sync.Map

func isRTSPAudioCopySource(rawURL string) bool {
	_, rawQuery, ok := strings.Cut(rawURL, "#")
	if !ok {
		return false
	}
	query := streams.ParseQuery(rawQuery)
	return len(query["video"]) > 0 && len(query["audio"]) == 1 && query.Get("audio") == "copy"
}

func replaceAudioCopyWithAAC(rawURL string) string {
	parts := strings.Split(rawURL, "#")
	for i := 1; i < len(parts); i++ {
		if parts[i] == "audio=copy" {
			parts[i] = "audio=aac"
			break
		}
	}
	return strings.Join(parts, "#")
}

func shouldFallbackRTSPAudio(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unsupported codec") &&
		(strings.Contains(message, "could not write header") || strings.Contains(message, "error opening output"))
}

func rtspAudioExecURL(rawURL string) string {
	rawURL = strings.TrimPrefix(rawURL, "ffmpeg:")
	return "exec:" + parseArgs(rawURL).String()
}

func getRTSPAudioCopyProducer(rawURL string, get producerGetter) (core.Producer, error) {
	if _, fallback := rtspAudioFallbacks.Load(rawURL); fallback {
		return get(rtspAudioExecURL(replaceAudioCopyWithAAC(rawURL)))
	}

	producer, err := get(rtspAudioExecURL(rawURL))
	if !shouldFallbackRTSPAudio(err) {
		return producer, err
	}

	log.Info().Msg("[ffmpeg] source audio is not supported by RTSP; retry with AAC")
	producer, err = get(rtspAudioExecURL(replaceAudioCopyWithAAC(rawURL)))
	if err == nil {
		rtspAudioFallbacks.Store(rawURL, struct{}{})
	}
	return producer, err
}

func newRTSPAudioCopyProducer(rawURL string) (core.Producer, error) {
	if _, err := Version(); err != nil {
		return nil, err
	}
	return getRTSPAudioCopyProducer(rawURL, streams.GetProducer)
}
