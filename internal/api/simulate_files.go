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

const (
	simulateUploadLimit             int64 = 20 << 30
	simulateUploadMultipartOverhead int64 = 1 << 20
)

var errSimulateUploadTooLarge = errors.New("upload exceeds the configured size limit")

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
	Entries      []simulateFileEntry `json:"entries"`
	Parent       string              `json:"parent"`
	Path         string              `json:"path"`
	RelativePath string              `json:"relative_path,omitempty"`
	CanGoUp      bool                `json:"can_go_up"`
	IsComputer   bool                `json:"is_computer,omitempty"`
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
	scope := strings.TrimSpace(r.URL.Query().Get("scope"))
	if scope == "upload" {
		simulateUploadDirectoriesHandler(w, rawPath)
		return
	}
	if scope != "" {
		http.Error(w, "unsupported backend browse scope", http.StatusBadRequest)
		return
	}

	if rawPath == "" && runtime.GOOS == "windows" {
		ResponseJSON(w, &simulateFilesResponse{
			Entries:    simulateWindowsDrives(),
			IsComputer: true,
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

	entries, err := simulateDirectoryEntries(dir, false)
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
		CanGoUp: parent != "" || runtime.GOOS == "windows" && filepath.VolumeName(dir)+string(filepath.Separator) == dir,
	})
}

func simulateUploadDirectoriesHandler(w http.ResponseWriter, rawPath string) {
	if simulateUploadDir == "" {
		http.Error(w, "simulate upload directory is not configured", http.StatusInternalServerError)
		return
	}
	if rawPath == "" && runtime.GOOS == "windows" {
		ResponseJSON(w, &simulateFilesResponse{
			Entries:    simulateWindowsDrives(),
			IsComputer: true,
		})
		return
	}

	dir, relativeDir, err := simulateUploadBrowseTarget(rawPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	entries, err := simulateDirectoryEntries(dir, true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	parent := filepath.Dir(dir)
	if parent == dir || filepath.VolumeName(dir)+string(filepath.Separator) == dir {
		parent = ""
	}

	ResponseJSON(w, &simulateFilesResponse{
		Entries:      entries,
		Parent:       filepath.ToSlash(parent),
		Path:         filepath.ToSlash(dir),
		RelativePath: filepath.ToSlash(relativeDir),
		CanGoUp:      parent != "" || runtime.GOOS == "windows" && filepath.VolumeName(dir)+string(filepath.Separator) == dir,
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

func simulateDirectoryEntries(dir string, directoriesOnly bool) ([]simulateFileEntry, error) {
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
		if directoriesOnly && !info.IsDir() {
			continue
		}
		if !directoriesOnly && !info.IsDir() && !isSimulateMediaFile(item.Name()) {
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
	simulateUploadHandlerWithLimit(w, r, simulateUploadLimit)
}

func simulateUploadHandlerWithLimit(w http.ResponseWriter, r *http.Request, uploadLimit int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if simulateUploadDir == "" {
		http.Error(w, "simulate upload directory is not configured", http.StatusInternalServerError)
		return
	}

	requestLimit := uploadLimit + simulateUploadMultipartOverhead
	if requestLimit < uploadLimit {
		requestLimit = uploadLimit
	}
	r.Body = http.MaxBytesReader(w, r.Body, requestLimit)
	reader, err := r.MultipartReader()
	if err != nil {
		writeSimulateUploadError(w, err)
		return
	}

	uploadPath := ""
	for {
		part, partErr := reader.NextPart()
		if errors.Is(partErr, io.EOF) {
			break
		}
		if partErr != nil {
			writeSimulateUploadError(w, partErr)
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
			response, uploadErr := saveSimulateUpload(part, uploadPath, uploadLimit)
			_ = part.Close()
			if uploadErr != nil {
				writeSimulateUploadError(w, uploadErr)
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

func writeSimulateUploadError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	var maxBytesErr *http.MaxBytesError
	if errors.Is(err, errSimulateUploadTooLarge) || errors.As(err, &maxBytesErr) {
		status = http.StatusRequestEntityTooLarge
	}
	http.Error(w, err.Error(), status)
}

func saveSimulateUpload(reader io.Reader, uploadPath string, uploadLimit int64) (*simulateUploadResponse, error) {
	dir, relativeDir, createDir, err := simulateUploadTarget(uploadPath)
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

	if createDir {
		if err = os.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}
		dir, err = secureSimulateUploadDir(dir)
		if err != nil {
			return nil, err
		}
	}

	temp, err := os.CreateTemp(dir, ".go2rtc-upload-*")
	if err != nil {
		return nil, err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)

	size, copyErr := io.Copy(temp, io.LimitReader(reader, uploadLimit+1))
	if copyErr == nil && size > uploadLimit {
		copyErr = fmt.Errorf("%w: limit is %d bytes", errSimulateUploadTooLarge, uploadLimit)
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

func simulateUploadTarget(uploadPath string) (dir, relativeDir string, createDir bool, err error) {
	rawPath := strings.TrimSpace(uploadPath)
	path := filepath.Clean(filepath.FromSlash(rawPath))
	if filepath.IsAbs(path) {
		dir, err = resolveExistingSimulateDirectory(path)
		if err != nil {
			return "", "", false, err
		}
		return dir, "", false, nil
	}
	if filepath.VolumeName(path) != "" {
		return "", "", false, errors.New("selected upload directory must be absolute")
	}

	relativeDir = path
	if relativeDir == "." {
		relativeDir = ""
	}

	dir = filepath.Clean(filepath.Join(simulateUploadDir, relativeDir))
	rel, err := filepath.Rel(simulateUploadDir, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", false, errors.New("path escapes the upload directory")
	}
	return dir, relativeDir, true, nil
}

func simulateUploadBrowseTarget(rawPath string) (dir, relativeDir string, err error) {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		dir = simulateUploadDir
	} else {
		path := filepath.Clean(filepath.FromSlash(rawPath))
		if filepath.IsAbs(path) || filepath.VolumeName(path) != "" {
			dir = path
		} else {
			if path == ".." || strings.HasPrefix(path, ".."+string(filepath.Separator)) {
				return "", "", errors.New("path escapes the upload directory")
			}
			dir = filepath.Join(simulateUploadDir, path)
		}
	}

	dir, err = resolveExistingSimulateDirectory(dir)
	if err != nil {
		return "", "", err
	}
	relativeDir, _ = relativePathWithinRoot(simulateUploadDir, dir)
	return dir, relativeDir, nil
}

func resolveExistingSimulateDirectory(dir string) (string, error) {
	dir = filepath.Clean(dir)
	info, err := os.Stat(dir)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("selected upload path must be a directory")
	}
	resolvedDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolvedDir), nil
}

func secureSimulateUploadDir(dir string) (string, error) {
	root, err := filepath.EvalSymlinks(simulateUploadDir)
	if err != nil {
		return "", err
	}
	resolvedDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", err
	}
	if _, err = relativePathWithinRoot(root, resolvedDir); err != nil {
		return "", err
	}
	return resolvedDir, nil
}

func relativePathWithinRoot(root, target string) (string, error) {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(target))
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes the upload directory")
	}
	if relative == "." {
		return "", nil
	}
	return relative, nil
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
