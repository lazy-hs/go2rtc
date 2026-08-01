package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/AlexxIT/go2rtc/internal/app"
)

const simulateUploadLimit int64 = 20 << 30

var simulateUploadDir string

var simulateMediaExtensions = map[string]struct{}{
	".3gp":  {},
	".avi":  {},
	".flv":  {},
	".m2ts": {},
	".m4v":  {},
	".mkv":  {},
	".mov":  {},
	".mp4":  {},
	".mpeg": {},
	".mpg":  {},
	".mts":  {},
	".ts":   {},
	".webm": {},
}

type simulateFileEntry struct {
	Name     string    `json:"name"`
	Path     string    `json:"path"`
	IsDir    bool      `json:"is_dir"`
	Size     int64     `json:"size,omitempty"`
	Modified time.Time `json:"modified"`
}

type simulateFilesResponse struct {
	Entries []simulateFileEntry `json:"entries"`
	Parent  string              `json:"parent"`
	Path    string              `json:"path"`
}

type simulateUploadResponse struct {
	Name         string `json:"name"`
	Path         string `json:"path"`
	RelativePath string `json:"relative_path"`
	Size         int64  `json:"size"`
}

func initSimulateFiles(configuredDir string) {
	dir, err := resolveSimulateUploadDir(configuredDir, app.ConfigPath)
	if err != nil {
		log.Error().Err(err).Msg("[api] simulate upload dir")
		return
	}

	if err = os.MkdirAll(dir, 0755); err != nil {
		log.Error().Err(err).Msg("[api] create simulate upload dir")
		return
	}

	simulateUploadDir = dir
	log.Info().Str("dir", dir).Msg("[api] simulate uploads")
}

func resolveSimulateUploadDir(configuredDir, configPath string) (string, error) {
	if configuredDir == "" {
		configuredDir = "static"
	}

	configuredDir = filepath.FromSlash(configuredDir)
	if !filepath.IsAbs(configuredDir) {
		baseDir := ""
		if configPath != "" {
			baseDir = filepath.Dir(configPath)
		} else {
			var err error
			baseDir, err = os.Getwd()
			if err != nil {
				return "", err
			}
		}
		configuredDir = filepath.Join(baseDir, configuredDir)
	}

	dir, err := filepath.Abs(configuredDir)
	if err != nil {
		return "", err
	}
	return filepath.Clean(dir), nil
}

func simulateFilesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rawPath := strings.TrimSpace(r.URL.Query().Get("path"))
	if rawPath == "" && runtime.GOOS == "windows" {
		ResponseJSON(w, &simulateFilesResponse{
			Entries: simulateWindowsDrives(),
			Parent:  "",
			Path:    "",
		})
		return
	}
	if rawPath == "" {
		rawPath = string(filepath.Separator)
	}

	dir := filepath.Clean(filepath.FromSlash(rawPath))
	if !filepath.IsAbs(dir) {
		http.Error(w, "backend browse path must be absolute", http.StatusBadRequest)
		return
	}

	info, err := os.Stat(dir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !info.IsDir() {
		http.Error(w, "backend browse path must be a directory", http.StatusBadRequest)
		return
	}

	entries, err := simulateDirectoryEntries(dir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	parent := filepath.Dir(dir)
	if parent == dir || filepath.VolumeName(dir)+string(filepath.Separator) == dir {
		parent = ""
	}

	ResponseJSON(w, &simulateFilesResponse{
		Entries: entries,
		Parent:  filepath.ToSlash(parent),
		Path:    filepath.ToSlash(dir),
	})
}

func simulateWindowsDrives() []simulateFileEntry {
	entries := make([]simulateFileEntry, 0, 8)
	for letter := 'A'; letter <= 'Z'; letter++ {
		root := fmt.Sprintf("%c:%c", letter, filepath.Separator)
		info, err := os.Stat(root)
		if err != nil || !info.IsDir() {
			continue
		}
		entries = append(entries, simulateFileEntry{
			Name:     fmt.Sprintf("%c:", letter),
			Path:     filepath.ToSlash(root),
			IsDir:    true,
			Modified: info.ModTime().UTC(),
		})
	}
	return entries
}

func simulateDirectoryEntries(dir string) ([]simulateFileEntry, error) {
	items, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	entries := make([]simulateFileEntry, 0, len(items))
	for _, item := range items {
		path := filepath.Join(dir, item.Name())
		info, statErr := os.Stat(path)
		if statErr != nil {
			continue
		}
		if !info.IsDir() && !isSimulateMediaFile(item.Name()) {
			continue
		}

		entry := simulateFileEntry{
			Name:     item.Name(),
			Path:     filepath.ToSlash(path),
			IsDir:    info.IsDir(),
			Modified: info.ModTime().UTC(),
		}
		if !entry.IsDir {
			entry.Size = info.Size()
		}
		entries = append(entries, entry)
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
	return entries, nil
}

func simulateUploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if simulateUploadDir == "" {
		http.Error(w, "simulate upload directory is not configured", http.StatusInternalServerError)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, simulateUploadLimit)
	reader, err := r.MultipartReader()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	uploadPath := ""
	for {
		part, partErr := reader.NextPart()
		if errors.Is(partErr, io.EOF) {
			break
		}
		if partErr != nil {
			http.Error(w, partErr.Error(), http.StatusBadRequest)
			return
		}

		switch {
		case part.FormName() == "path" && part.FileName() == "":
			value, readErr := io.ReadAll(io.LimitReader(part, 4097))
			_ = part.Close()
			if readErr != nil {
				http.Error(w, readErr.Error(), http.StatusBadRequest)
				return
			}
			if len(value) > 4096 {
				http.Error(w, "upload path is too long", http.StatusBadRequest)
				return
			}
			uploadPath = strings.TrimSpace(string(value))
		case part.FormName() == "file" && part.FileName() != "":
			response, uploadErr := saveSimulateUpload(part, uploadPath)
			_ = part.Close()
			if uploadErr != nil {
				http.Error(w, uploadErr.Error(), http.StatusBadRequest)
				return
			}
			ResponseJSON(w, response)
			return
		default:
			_, _ = io.Copy(io.Discard, part)
			_ = part.Close()
		}
	}

	http.Error(w, "upload file is required", http.StatusBadRequest)
}

func saveSimulateUpload(reader io.Reader, uploadPath string) (*simulateUploadResponse, error) {
	dir, relativeDir, err := simulateUploadTarget(uploadPath)
	if err != nil {
		return nil, err
	}

	filePart, ok := reader.(interface{ FileName() string })
	if !ok {
		return nil, errors.New("upload file name is required")
	}
	name := filepath.Base(strings.ReplaceAll(filePart.FileName(), "\\", "/"))
	name = strings.ReplaceAll(name, "#", "_")
	if name == "" || name == "." {
		return nil, errors.New("upload file name is required")
	}
	if !isSimulateMediaFile(name) {
		return nil, fmt.Errorf("unsupported media file: %s", strings.ToLower(filepath.Ext(name)))
	}

	if err = os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	temp, err := os.CreateTemp(dir, ".go2rtc-upload-*")
	if err != nil {
		return nil, err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)

	size, copyErr := io.Copy(temp, io.LimitReader(reader, simulateUploadLimit+1))
	if copyErr == nil && size > simulateUploadLimit {
		copyErr = fmt.Errorf("upload exceeds %d bytes", simulateUploadLimit)
	}
	if copyErr == nil {
		copyErr = temp.Sync()
	}
	if closeErr := temp.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		return nil, copyErr
	}
	if err = os.Chmod(tempPath, 0644); err != nil {
		return nil, err
	}

	publishedName, publishedPath, err := publishSimulateUpload(tempPath, dir, name)
	if err != nil {
		return nil, err
	}
	relativePath := publishedName
	if relativeDir != "" {
		relativePath = filepath.Join(relativeDir, publishedName)
	}

	return &simulateUploadResponse{
		Name:         publishedName,
		Path:         filepath.ToSlash(publishedPath),
		RelativePath: filepath.ToSlash(relativePath),
		Size:         size,
	}, nil
}

func simulateUploadTarget(uploadPath string) (dir, relativeDir string, err error) {
	relativeDir = filepath.Clean(filepath.FromSlash(strings.TrimSpace(uploadPath)))
	if relativeDir == "." {
		relativeDir = ""
	}
	if filepath.IsAbs(relativeDir) || filepath.VolumeName(relativeDir) != "" {
		return "", "", errors.New("path escapes the upload directory")
	}

	dir = filepath.Clean(filepath.Join(simulateUploadDir, relativeDir))
	rel, err := filepath.Rel(simulateUploadDir, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", errors.New("path escapes the upload directory")
	}
	return dir, relativeDir, nil
}

func publishSimulateUpload(tempPath, dir, name string) (publishedName, publishedPath string, err error) {
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for index := 1; ; index++ {
		publishedName = name
		if index > 1 {
			publishedName = fmt.Sprintf("%s-%d%s", base, index, ext)
		}
		publishedPath = filepath.Join(dir, publishedName)
		err = os.Link(tempPath, publishedPath)
		if err == nil {
			return publishedName, publishedPath, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return "", "", err
		}
	}
}

func isSimulateMediaFile(name string) bool {
	_, ok := simulateMediaExtensions[strings.ToLower(filepath.Ext(name))]
	return ok
}
