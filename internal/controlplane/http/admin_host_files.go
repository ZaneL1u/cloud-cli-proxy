package http

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	nethttp "net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var sensitiveDirs = []string{
	"/proc", "/sys", "/dev", "/boot", "/etc/ssh", "/root/.ssh",
}

func isAbsolutePath(path string) bool {
	if strings.HasPrefix(path, "/") {
		return true
	}
	if len(path) >= 3 && path[1] == ':' && (path[2] == '\\' || path[2] == '/') {
		c := path[0]
		return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
	}
	return false
}

type AdminHostFilesHandler struct {
	logger *slog.Logger
}

func NewAdminHostFilesHandler(logger *slog.Logger) *AdminHostFilesHandler {
	return &AdminHostFilesHandler{logger: logger}
}

type hostFileEntry struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	IsDir  bool   `json:"is_dir"`
	Size   int64  `json:"size"`
	ModTime string `json:"mod_time"`
}

func (h *AdminHostFilesHandler) List() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		path := r.URL.Query().Get("path")
		if path == "" {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "path parameter is required"})
			return
		}
		if !isAbsolutePath(path) {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "path must be absolute"})
			return
		}
		cleaned := filepath.Clean(path)
		if strings.Contains(cleaned, "..") {
			writeJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "path traversal not allowed"})
			return
		}
		if strings.HasPrefix(path, "/") {
			for _, blocked := range sensitiveDirs {
				if strings.HasPrefix(cleaned, blocked) {
					writeJSON(w, nethttp.StatusForbidden, map[string]string{"error": "access to sensitive directory denied"})
					return
				}
			}
		}

		hostID := r.URL.Query().Get("host_id")

		var entries []hostFileEntry
		var err error
		if hostID != "" {
			entries, err = listContainerFiles(r.Context(), hostID, cleaned)
		} else {
			entries, err = listLocalFiles(cleaned)
		}
		if err != nil {
			h.logger.Error("list files failed", "path", cleaned, "host_id", hostID, "error", err)
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "unable to read directory"})
			return
		}

		writeJSON(w, nethttp.StatusOK, map[string]any{"entries": entries})
	})
}

func listLocalFiles(dir string) ([]hostFileEntry, error) {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []hostFileEntry{}, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return []hostFileEntry{}, nil
	}

	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []hostFileEntry{}, nil
		}
		return nil, err
	}

	var entries []hostFileEntry
	for _, e := range dirEntries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		entries = append(entries, hostFileEntry{
			Name:    e.Name(),
			Path:    filepath.Join(dir, e.Name()),
			IsDir:   e.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime().UTC().Format(time.RFC3339),
		})
	}
	return entries, nil
}

func listContainerFiles(ctx context.Context, hostID, dir string) ([]hostFileEntry, error) {
	containerName := "cloudproxy-" + hostID
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	script := fmt.Sprintf(
		`cd "$1" 2>/dev/null || exit 0; for f in * .*; do [ "$f" = "." ] && continue; [ "$f" = ".." ] && continue; [ -e "$f" ] || continue; if [ -d "$f" ]; then t="d"; else t="f"; fi; s=$(stat -c%%s "$f" 2>/dev/null || stat -f%%z "$f" 2>/dev/null || echo 0); m=$(stat -c%%Y "$f" 2>/dev/null || stat -f%%m "$f" 2>/dev/null || echo 0); printf "%%s\t%%s\t%%s\t%%s\n" "$t" "$f" "$s" "$m"; done`,
	)
	cmd := exec.CommandContext(ctx, "docker", "exec", containerName, "sh", "-c", script, "--", dir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return []hostFileEntry{}, nil
		}
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("timeout listing container files")
		}
		return nil, fmt.Errorf("docker exec failed: %w, output: %s", err, string(output))
	}

	return parseTabOutput(dir, string(output)), nil
}

func parseTabOutput(parentDir, output string) []hostFileEntry {
	var entries []hostFileEntry
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 4)
		if len(parts) < 4 {
			continue
		}
		entryType := parts[0]
		name := parts[1]
		size, _ := strconv.ParseInt(parts[2], 10, 64)
		mtimeUnix, _ := strconv.ParseInt(parts[3], 10, 64)

		modTime := ""
		if mtimeUnix > 0 {
			modTime = time.Unix(mtimeUnix, 0).UTC().Format(time.RFC3339)
		}

		entries = append(entries, hostFileEntry{
			Name:    name,
			Path:    filepath.Join(parentDir, name),
			IsDir:   entryType == "d",
			Size:    size,
			ModTime: modTime,
		})
	}
	return entries
}

// Keep for backward compatibility with tests
func writeListEntries(w nethttp.ResponseWriter, entries []string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(nethttp.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"entries": entries})
}
