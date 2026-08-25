package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var errSimulateFolderPickerCancelled = errors.New("folder selection cancelled")

var (
	simulateFolderPickerMu        sync.Mutex
	simulateFolderPickerAvailable = simulateNativeFolderPickerAvailable
	runSimulateFolderPicker       = simulatePickNativeFolder
	simulateInterfaceAddrs        = net.InterfaceAddrs
)

type simulateFolderPickerResponse struct {
	Path string `json:"path"`
}

func simulateFolderPickerHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !simulateFolderPickerAvailable() {
		http.Error(w, "native folder picker is not supported on this platform", http.StatusNotImplemented)
		return
	}
	if !simulateLocalRequest(r) {
		http.Error(w, "native folder picker is only available to local clients", http.StatusForbidden)
		return
	}
	if !simulateFolderPickerMu.TryLock() {
		http.Error(w, "a folder picker is already open", http.StatusConflict)
		return
	}
	defer simulateFolderPickerMu.Unlock()

	initialDir := strings.TrimSpace(r.URL.Query().Get("path"))
	if initialDir == "" {
		initialDir = simulateUploadDir
	} else if _, err := resolveExistingSimulateDirectory(filepath.FromSlash(initialDir)); err != nil {
		initialDir = simulateUploadDir
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	selectedDir, err := runSimulateFolderPicker(ctx, initialDir)
	if errors.Is(err, errSimulateFolderPickerCancelled) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			http.Error(w, "native folder picker timed out", http.StatusGatewayTimeout)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	selectedDir, err = resolveExistingSimulateDirectory(selectedDir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ResponseJSON(w, &simulateFolderPickerResponse{Path: filepathToSlash(selectedDir)})
}

func simulateLocalRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		host = strings.Trim(strings.TrimSpace(r.RemoteAddr), "[]")
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() {
		return true
	}

	addresses, err := simulateInterfaceAddrs()
	if err != nil {
		return false
	}
	for _, address := range addresses {
		var localIP net.IP
		switch value := address.(type) {
		case *net.IPNet:
			localIP = value.IP
		case *net.IPAddr:
			localIP = value.IP
		}
		if localIP != nil && localIP.Equal(ip) {
			return true
		}
	}
	return false
}

func filepathToSlash(path string) string {
	return strings.ReplaceAll(path, "\\", "/")
}
