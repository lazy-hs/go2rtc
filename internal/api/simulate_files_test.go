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

func TestSimulateUploadHandlerUsesConfiguredSubdirectory(t *testing.T) {
	root := t.TempDir()
	withSimulateUploadDir(t, root)

	res := performSimulateUpload(t, "camera-a/2026", "sample.mp4", []byte("video-content"))
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())

	var uploaded simulateUploadResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &uploaded))
	require.Equal(t, "camera-a/2026/sample.mp4", uploaded.RelativePath)
	require.Equal(t, filepath.Join(root, "camera-a", "2026", "sample.mp4"), filepath.FromSlash(uploaded.Path))
	content, err := os.ReadFile(filepath.Join(root, "camera-a", "2026", "sample.mp4"))
	require.NoError(t, err)
	require.Equal(t, []byte("video-content"), content)
}

func TestSimulateUploadHandlerUsesSelectedAbsoluteDirectory(t *testing.T) {
	withSimulateUploadDir(t, t.TempDir())
	selectedDir := t.TempDir()

	res := performSimulateUpload(t, selectedDir, "sample.mp4", []byte("video-content"))
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())

	var uploaded simulateUploadResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &uploaded))
	require.Equal(t, filepath.Join(selectedDir, "sample.mp4"), filepath.FromSlash(uploaded.Path))
	content, err := os.ReadFile(filepath.Join(selectedDir, "sample.mp4"))
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

func TestSimulateUploadHandlerRejectsOversizedFile(t *testing.T) {
	root := t.TempDir()
	withSimulateUploadDir(t, root)

	res := performSimulateUploadWithLimit(t, "", "sample.mp4", bytes.Repeat([]byte("v"), 2048), 1024)
	require.Equal(t, http.StatusRequestEntityTooLarge, res.Code)
	require.Contains(t, res.Body.String(), "upload exceeds the configured size limit")
	require.NoFileExists(t, filepath.Join(root, "sample.mp4"))

	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Empty(t, entries, "oversized uploads should not leave temporary files behind")
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

func TestSimulateFilesHandlerBrowsesUploadDirectories(t *testing.T) {
	root := t.TempDir()
	withSimulateUploadDir(t, root)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "camera-a", "2026"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "clip.mp4"), []byte("video"), 0644))

	query := url.Values{"scope": []string{"upload"}, "path": []string{root}}
	req := httptest.NewRequest(http.MethodGet, "/api/simulate/files?"+query.Encode(), nil)
	res := httptest.NewRecorder()
	simulateFilesHandler(res, req)
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())

	var response simulateFilesResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &response))
	require.Equal(t, filepath.ToSlash(root), response.Path)
	require.Empty(t, response.RelativePath)
	require.Equal(t, filepath.ToSlash(filepath.Dir(root)), response.Parent)
	require.True(t, response.CanGoUp)
	require.Len(t, response.Entries, 1)
	require.Equal(t, "camera-a", response.Entries[0].Name)
	require.True(t, response.Entries[0].IsDir)

	query = url.Values{"scope": []string{"upload"}, "path": []string{"camera-a/2026"}}
	req = httptest.NewRequest(http.MethodGet, "/api/simulate/files?"+query.Encode(), nil)
	res = httptest.NewRecorder()
	simulateFilesHandler(res, req)
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	response = simulateFilesResponse{}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &response))
	require.Equal(t, "camera-a/2026", response.RelativePath)
	require.Equal(t, filepath.ToSlash(filepath.Join(root, "camera-a")), response.Parent)
}

func TestSimulateFilesHandlerUploadScopeWindowsComputerAndDriveRoot(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows drive navigation")
	}

	root := t.TempDir()
	withSimulateUploadDir(t, root)
	req := httptest.NewRequest(http.MethodGet, "/api/simulate/files?scope=upload", nil)
	res := httptest.NewRecorder()
	simulateFilesHandler(res, req)
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())

	var response simulateFilesResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &response))
	require.True(t, response.IsComputer)
	require.False(t, response.CanGoUp)
	require.Empty(t, response.Path)
	require.NotEmpty(t, response.Entries)

	volumeRoot := filepath.VolumeName(root) + string(filepath.Separator)
	query := url.Values{"scope": []string{"upload"}, "path": []string{volumeRoot}}
	req = httptest.NewRequest(http.MethodGet, "/api/simulate/files?"+query.Encode(), nil)
	res = httptest.NewRecorder()
	simulateFilesHandler(res, req)
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	response = simulateFilesResponse{}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &response))
	require.False(t, response.IsComputer)
	require.True(t, response.CanGoUp)
	require.Empty(t, response.Parent)
	require.Equal(t, filepath.ToSlash(volumeRoot), response.Path)
}

func TestSimulateFilesHandlerBrowsesParentOutsideUploadRoot(t *testing.T) {
	rootParent := t.TempDir()
	root := filepath.Join(rootParent, "static")
	require.NoError(t, os.Mkdir(root, 0755))
	withSimulateUploadDir(t, root)

	query := url.Values{"scope": []string{"upload"}, "path": []string{rootParent}}
	req := httptest.NewRequest(http.MethodGet, "/api/simulate/files?"+query.Encode(), nil)
	res := httptest.NewRecorder()
	simulateFilesHandler(res, req)
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())

	var response simulateFilesResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &response))
	require.Equal(t, filepath.ToSlash(rootParent), response.Path)
	require.Equal(t, filepath.ToSlash(filepath.Dir(rootParent)), response.Parent)
	require.Len(t, response.Entries, 1)
	require.Equal(t, "static", response.Entries[0].Name)
}

func TestSimulateFilesHandlerRejectsUploadScopeEscape(t *testing.T) {
	withSimulateUploadDir(t, t.TempDir())

	query := url.Values{"scope": []string{"upload"}, "path": []string{"../outside"}}
	req := httptest.NewRequest(http.MethodGet, "/api/simulate/files?"+query.Encode(), nil)
	res := httptest.NewRecorder()
	simulateFilesHandler(res, req)
	require.Equal(t, http.StatusBadRequest, res.Code)
	require.Contains(t, res.Body.String(), "path escapes the upload directory")
}

func TestSimulateUploadHandlerRejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks normally requires elevated Windows privileges")
	}

	root := t.TempDir()
	outside := t.TempDir()
	withSimulateUploadDir(t, root)
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "outside-link")))

	res := performSimulateUpload(t, "outside-link", "sample.mp4", []byte("video"))
	require.Equal(t, http.StatusBadRequest, res.Code)
	require.Contains(t, res.Body.String(), "path escapes the upload directory")
	require.NoFileExists(t, filepath.Join(outside, "sample.mp4"))
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
		require.True(t, response.IsComputer)
		require.False(t, response.CanGoUp)
		require.NotEmpty(t, response.Entries)
		for _, entry := range response.Entries {
			require.True(t, entry.IsDir)
			require.True(t, filepath.IsAbs(filepath.FromSlash(entry.Path)))
		}
	} else {
		require.Equal(t, "/", response.Path)
		require.Empty(t, response.Parent)
		require.False(t, response.CanGoUp)
	}
}

func withSimulateUploadDir(t *testing.T, dir string) {
	t.Helper()
	oldDir := simulateUploadDir
	simulateUploadDir = filepath.Clean(dir)
	t.Cleanup(func() { simulateUploadDir = oldDir })
}

func performSimulateUpload(t *testing.T, path, name string, content []byte) *httptest.ResponseRecorder {
	return performSimulateUploadWithLimit(t, path, name, content, simulateUploadLimit)
}

func performSimulateUploadWithLimit(t *testing.T, path, name string, content []byte, uploadLimit int64) *httptest.ResponseRecorder {
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
	simulateUploadHandlerWithLimit(res, req, uploadLimit)
	return res
}

func TestSimulateFilesHandlerRejectsRelativePath(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/simulate/files?path="+url.QueryEscape("relative/path"), nil)
	res := httptest.NewRecorder()
	simulateFilesHandler(res, req)
	require.Equal(t, http.StatusBadRequest, res.Code)
	require.True(t, strings.Contains(res.Body.String(), "must be absolute"))
}
