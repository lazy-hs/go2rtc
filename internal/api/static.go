package api

import (
	"net/http"
	"strings"

	"github.com/AlexxIT/go2rtc/www"
)

func initStatic(staticDir string) {
	var root http.FileSystem
	if staticDir != "" {
		log.Info().Str("dir", staticDir).Msg("[api] serve static")
		root = http.Dir(staticDir)
	} else {
		root = http.FS(www.Static)
	}

	base := len(basePath)
	fileServer := http.FileServer(root)
	defaultPath := strings.TrimRight(basePath, "/") + "/"
	defaultPage := defaultPath + "simulate.html"
	legacyStreamsPage := defaultPath + "index.html"
	streamsPage := defaultPath + "streams.html"

	HandleFunc("", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == defaultPath {
			http.Redirect(w, r, defaultPage, http.StatusFound)
			return
		}
		if r.URL.Path == legacyStreamsPage {
			http.Redirect(w, r, streamsPage, http.StatusFound)
			return
		}
		if base > 0 {
			r.URL.Path = r.URL.Path[base:]
		}
		fileServer.ServeHTTP(w, r)
	})
}
