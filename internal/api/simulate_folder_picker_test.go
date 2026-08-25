package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSimulateFolderPickerHandler(t *testing.T) {
	selectedDir := t.TempDir()
	withSimulateFolderPicker(t, func(context.Context, string) (string, error) {
		return selectedDir, nil
	})

	req := httptest.NewRequest(http.MethodPost, "/api/simulate/folder-picker", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	res := httptest.NewRecorder()
	simulateFolderPickerHandler(res, req)
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())

	var response simulateFolderPickerResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &response))
	require.Equal(t, filepath.ToSlash(selectedDir), response.Path)
}

func TestSimulateFolderPickerHandlerCancelled(t *testing.T) {
	withSimulateFolderPicker(t, func(context.Context, string) (string, error) {
		return "", errSimulateFolderPickerCancelled
	})

	req := httptest.NewRequest(http.MethodPost, "/api/simulate/folder-picker", nil)
	req.RemoteAddr = "[::1]:54321"
	res := httptest.NewRecorder()
	simulateFolderPickerHandler(res, req)
	require.Equal(t, http.StatusNoContent, res.Code)
}

func TestSimulateFolderPickerHandlerRejectsRemoteClient(t *testing.T) {
	withSimulateFolderPicker(t, func(context.Context, string) (string, error) {
		t.Fatal("remote requests must not open the native picker")
		return "", nil
	})

	req := httptest.NewRequest(http.MethodPost, "/api/simulate/folder-picker", nil)
	req.RemoteAddr = "192.0.2.10:54321"
	res := httptest.NewRecorder()
	simulateFolderPickerHandler(res, req)
	require.Equal(t, http.StatusForbidden, res.Code)
}

func TestSimulateFolderPickerHandlerAllowsLocalInterfaceAddress(t *testing.T) {
	withSimulateInterfaceAddrs(t, []net.Addr{
		&net.IPNet{IP: net.ParseIP("192.168.73.241"), Mask: net.CIDRMask(24, 32)},
	})
	selectedDir := t.TempDir()
	withSimulateFolderPicker(t, func(context.Context, string) (string, error) {
		return selectedDir, nil
	})

	req := httptest.NewRequest(http.MethodPost, "/api/simulate/folder-picker", nil)
	req.RemoteAddr = "192.168.73.241:54321"
	res := httptest.NewRecorder()
	simulateFolderPickerHandler(res, req)
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
}

func TestSimulateFolderPickerHandlerFallsBackFromMissingInitialDirectory(t *testing.T) {
	uploadDir := t.TempDir()
	withSimulateUploadDir(t, uploadDir)

	missingDir := filepath.Join(t.TempDir(), "missing")
	withSimulateFolderPicker(t, func(_ context.Context, initialDir string) (string, error) {
		require.Equal(t, filepath.Clean(uploadDir), filepath.Clean(initialDir))
		return uploadDir, nil
	})

	req := httptest.NewRequest(http.MethodPost, "/api/simulate/folder-picker?path="+url.QueryEscape(missingDir), nil)
	req.RemoteAddr = "127.0.0.1:54321"
	res := httptest.NewRecorder()
	simulateFolderPickerHandler(res, req)
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
}

func withSimulateFolderPicker(t *testing.T, picker func(context.Context, string) (string, error)) {
	t.Helper()
	oldAvailable := simulateFolderPickerAvailable
	oldPicker := runSimulateFolderPicker
	simulateFolderPickerAvailable = func() bool { return true }
	runSimulateFolderPicker = picker
	t.Cleanup(func() {
		simulateFolderPickerAvailable = oldAvailable
		runSimulateFolderPicker = oldPicker
	})
}

func withSimulateInterfaceAddrs(t *testing.T, addresses []net.Addr) {
	t.Helper()
	old := simulateInterfaceAddrs
	simulateInterfaceAddrs = func() ([]net.Addr, error) { return addresses, nil }
	t.Cleanup(func() { simulateInterfaceAddrs = old })
}
