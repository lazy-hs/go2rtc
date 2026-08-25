package device

import (
	"net/http"
	"net/url"
	"strconv"
	"sync"

	"github.com/AlexxIT/go2rtc/internal/api"
)

func Init(bin string) {
	Bin = bin

	api.HandleFunc("api/ffmpeg/devices", apiDevices)
}

func GetInput(src string) string {
	query, err := url.ParseQuery(src)
	if err != nil {
		return ""
	}

	runonce.Do(initDevices)

	return queryToInput(query)
}

var Bin string

var videos, audios []string
var streams []*api.Source
var runonce sync.Once

func apiDevices(w http.ResponseWriter, r *http.Request) {
	runonce.Do(initDevices)
	writeDeviceSources(w, streams)
}

func writeDeviceSources(w http.ResponseWriter, sources []*api.Source) {
	// No capture devices is a valid result on headless Linux hosts and in
	// containers, so return an empty collection instead of a noisy 404.
	if sources == nil {
		sources = []*api.Source{}
	}
	api.ResponseJSON(w, struct {
		Sources []*api.Source `json:"sources"`
	}{Sources: sources})
}

func indexToItem(items []string, index string) string {
	if i, err := strconv.Atoi(index); err == nil && i < len(items) {
		return items[i]
	}
	return index
}
