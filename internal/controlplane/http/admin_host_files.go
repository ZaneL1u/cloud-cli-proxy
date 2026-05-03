package http

import (
	"encoding/json"
	"log/slog"
	nethttp "net/http"
	"os"
	"path/filepath"
	"strings"
)

var sensitiveDirs = []string{
	"/proc", "/sys", "/dev", "/boot", "/etc/ssh", "/root/.ssh",
}

type AdminHostFilesHandler struct {
	logger *slog.Logger
}

func NewAdminHostFilesHandler(logger *slog.Logger) *AdminHostFilesHandler {
	return &AdminHostFilesHandler{logger: logger}
}

func (h *AdminHostFilesHandler) List() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		path := r.URL.Query().Get("path")
		if path == "" {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "path parameter is required"})
			return
		}
		if !strings.HasPrefix(path, "/") {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "path must be absolute"})
			return
		}
		cleaned := filepath.Clean(path)
		if strings.Contains(cleaned, "..") {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "path traversal not allowed"})
			return
		}
		for _, blocked := range sensitiveDirs {
			if strings.HasPrefix(cleaned, blocked) {
				writeJSON(w, nethttp.StatusForbidden, map[string]string{"error": "access to sensitive directory denied"})
				return
			}
		}

		info, err := os.Stat(cleaned)
		if err != nil {
			if os.IsNotExist(err) {
				writeJSON(w, nethttp.StatusNotFound, map[string]string{"error": "path not found"})
				return
			}
			h.logger.Error("stat path failed", "path", cleaned, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "unable to read path"})
			return
		}
		if !info.IsDir() {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "path is not a directory"})
			return
		}

		entries, err := os.ReadDir(cleaned)
		if err != nil {
			h.logger.Error("read dir failed", "path", cleaned, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "unable to read directory"})
			return
		}

		var dirs []string
		for _, e := range entries {
			if e.IsDir() {
				dirs = append(dirs, e.Name())
			}
		}

		writeJSON(w, nethttp.StatusOK, map[string]any{"entries": dirs})
	})
}

type listEntriesResponse struct {
	Entries []string `json:"entries"`
}

func writeListEntries(w nethttp.ResponseWriter, entries []string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(nethttp.StatusOK)
	_ = json.NewEncoder(w).Encode(listEntriesResponse{Entries: entries})
}
