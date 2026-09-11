package api

import "net/http"

var configEnabled = true

func configHandlerWithVisibility(w http.ResponseWriter, r *http.Request) {
	if !configEnabled {
		http.Error(w, "config page disabled", http.StatusGone)
		return
	}
	configHandler(w, r)
}
