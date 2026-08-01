package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveSimulateUploadDirDefaultsNextToConfig(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config", "go2rtc.yaml")
	dir, err := resolveSimulateUploadDir("", configPath)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(filepath.Dir(configPath), "static"), dir)
}

func TestSimulateUploadHandler(t *testing.T) {
	withSimulateUploadDir(t, t.TempDir())

	res := performSimulateUpload(t, "", "sample.mp4", []byte("video-content"))
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())

	var uploaded simulateUploadResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &uploaded))
	require.Equal(t, "sample.mp4", uploaded.Name)
	require.Equal(t, "sample.mp4", uploaded.RelativePath)
	require.EqualValues(t, len("video-content"), uploaded.Size)
	content, err := os.ReadFile(filepath.FromSlash(uploaded.Path))
	require.NoError(t, err)
	require.Equal(t, []byte("video-content"), content)
}

func TestSimulateUploadHandlerNumbersDuplicateNames(t *testing.T) {
	withSimulateUploadDir(t, t.TempDir())

	first := performSimulateUpload(t, "", "sample.mp4", []byte("first"))
	second := performSimulateUpload(t, "", "sample.mp4", []byte("second"))
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	require.Equal(t, http.StatusOK, second.Code, second.Body.String())

	var uploaded simulateUploadResponse
	require.NoError(t, json.Unmarshal(second.Body.Bytes(), &uploaded))
	require.Equal(t, "sample-2.mp4", uploaded.Name)
	content, err := os.ReadFile(filepath.FromSlash(uploaded.Path))
	require.NoError(t, err)
	require.Equal(t, []byte("second"), content)
}

func TestSimulateUploadHandlerRejectsUnsupportedExtension(t *testing.T) {
	withSimulateUploadDir(t, t.TempDir())

	res := performSimulateUpload(t, "", "page.html", []byte("not-video"))
	require.Equal(t, http.StatusBadRequest, res.Code)
	require.Contains(t, res.Body.String(), "unsupported media file: .html")
}

func TestSimulateUploadHandlerRejectsTraversal(t *testing.T) {
	withSimulateUploadDir(t, t.TempDir())

	res := performSimulateUpload(t, "../outside", "sample.mp4", []byte("video"))
	require.Equal(t, http.StatusBadRequest, res.Code)
	require.Contains(t, res.Body.String(), "path escapes the upload directory")
}

func TestSimulateFilesHandlerBrowsesOutsideUploadRoot(t *testing.T) {
	withSimulateUploadDir(t, t.TempDir())
	browseDir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(browseDir, "folder"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(browseDir, "clip.mp4"), []byte("video"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(browseDir, "notes.txt"), []byte("hidden"), 0644))

	query := url.Values{"path": []string{browseDir}}
	req := httptest.NewRequest(http.MethodGet, "/api/simulate/files?"+query.Encode(), nil)
	res := httptest.NewRecorder()
	simulateFilesHandler(res, req)
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())

	var response simulateFilesResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &response))
	require.Equal(t, filepath.ToSlash(filepath.Clean(browseDir)), response.Path)
	require.Len(t, response.Entries, 2)
	require.Equal(t, "folder", response.Entries[0].Name)
	require.True(t, response.Entries[0].IsDir)
	require.Equal(t, "clip.mp4", response.Entries[1].Name)
	require.False(t, response.Entries[1].IsDir)
}

func TestSimulateFilesHandlerInitialLocation(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/simulate/files", nil)
	res := httptest.NewRecorder()
	simulateFilesHandler(res, req)
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())

	var response simulateFilesResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &response))
	if runtime.GOOS == "windows" {
		require.Empty(t, response.Path)
		require.NotEmpty(t, response.Entries)
		for _, entry := range response.Entries {
			require.True(t, entry.IsDir)
			require.True(t, filepath.IsAbs(filepath.FromSlash(entry.Path)))
		}
	} else {
		require.Equal(t, "/", response.Path)
		require.Empty(t, response.Parent)
	}
}

func withSimulateUploadDir(t *testing.T, dir string) {
	t.Helper()
	oldDir := simulateUploadDir
	simulateUploadDir = filepath.Clean(dir)
	t.Cleanup(func() { simulateUploadDir = oldDir })
}

func performSimulateUpload(t *testing.T, path, name string, content []byte) *httptest.ResponseRecorder {
	t.Helper()

	body := bytes.NewBuffer(nil)
	writer := multipart.NewWriter(body)
	require.NoError(t, writer.WriteField("path", path))
	part, err := writer.CreateFormFile("file", name)
	require.NoError(t, err)
	_, err = part.Write(content)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req := httptest.NewRequest(http.MethodPost, "/api/simulate/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	res := httptest.NewRecorder()
	simulateUploadHandler(res, req)
	return res
}

func TestSimulateFilesHandlerRejectsRelativePath(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/simulate/files?path="+url.QueryEscape("relative/path"), nil)
	res := httptest.NewRecorder()
	simulateFilesHandler(res, req)
	require.Equal(t, http.StatusBadRequest, res.Code)
	require.True(t, strings.Contains(res.Body.String(), "must be absolute"))
}
