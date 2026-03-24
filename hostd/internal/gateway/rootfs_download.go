package gateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
)

type rootfsDownloadParams struct {
	URL      string `json:"url"`
	Checksum string `json:"checksum"`
	DestPath string `json:"destPath"`
	OwnerUID int    `json:"ownerUid"`
	OwnerGID int    `json:"ownerGid"`
}

// handleRootfsDownload downloads a rootfs image, verifies its checksum, and
// moves it to the destination path.
func (s *Server) handleRootfsDownload(ctx context.Context, params json.RawMessage) Response {
	var p rootfsDownloadParams
	if err := json.Unmarshal(params, &p); err != nil {
		return Response{OK: false, Error: "invalid params: " + err.Error(), Code: "PARSE_ERROR"}
	}
	if p.URL == "" {
		return Response{OK: false, Error: "url is required", Code: "VALIDATION_ERROR"}
	}
	if p.DestPath == "" {
		return Response{OK: false, Error: "destPath is required", Code: "VALIDATION_ERROR"}
	}

	// Ensure destination directory exists.
	destDir := filepath.Dir(p.DestPath)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return Response{OK: false, Error: fmt.Sprintf("create dest dir: %s", err), Code: "INTERNAL_ERROR"}
	}

	// Download to a temp file in the same directory (for atomic rename).
	tmpFile, err := os.CreateTemp(destDir, "rootfs-download-*.tmp")
	if err != nil {
		return Response{OK: false, Error: fmt.Sprintf("create temp file: %s", err), Code: "INTERNAL_ERROR"}
	}
	tmpPath := tmpFile.Name()
	defer func() {
		tmpFile.Close()
		os.Remove(tmpPath) // clean up on error
	}()

	// Download the file.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.URL, nil)
	if err != nil {
		return Response{OK: false, Error: fmt.Sprintf("create request: %s", err), Code: "DOWNLOAD_ERROR"}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Response{OK: false, Error: fmt.Sprintf("download: %s", err), Code: "DOWNLOAD_ERROR"}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Response{OK: false, Error: fmt.Sprintf("download failed: HTTP %d", resp.StatusCode), Code: "DOWNLOAD_ERROR"}
	}

	// Write to temp file while computing SHA256.
	hasher := sha256.New()
	writer := io.MultiWriter(tmpFile, hasher)
	written, err := io.Copy(writer, resp.Body)
	if err != nil {
		return Response{OK: false, Error: fmt.Sprintf("write download: %s", err), Code: "DOWNLOAD_ERROR"}
	}
	if err := tmpFile.Close(); err != nil {
		return Response{OK: false, Error: fmt.Sprintf("close temp file: %s", err), Code: "INTERNAL_ERROR"}
	}

	checksum := hex.EncodeToString(hasher.Sum(nil))

	// Verify checksum if provided.
	if p.Checksum != "" && checksum != p.Checksum {
		os.Remove(tmpPath)
		return Response{
			OK:    false,
			Error: fmt.Sprintf("checksum mismatch: expected %s, got %s", p.Checksum, checksum),
			Code:  "CHECKSUM_ERROR",
		}
	}

	// Atomic rename to destination.
	if err := os.Rename(tmpPath, p.DestPath); err != nil {
		return Response{OK: false, Error: fmt.Sprintf("rename to dest: %s", err), Code: "INTERNAL_ERROR"}
	}

	// Chown if requested.
	if p.OwnerUID > 0 || p.OwnerGID > 0 {
		uid := p.OwnerUID
		gid := p.OwnerGID
		if gid <= 0 {
			gid = uid
		}
		if err := os.Chown(p.DestPath, uid, gid); err != nil {
			slog.Warn("chown rootfs download failed", "error", err)
		}
	}

	slog.Info("rootfs downloaded",
		"url", p.URL,
		"dest", p.DestPath,
		"size", written,
		"checksum", checksum,
	)

	return Response{
		OK: true,
		VM: map[string]any{
			"destPath": p.DestPath,
			"checksum": checksum,
			"size":     written,
		},
	}
}
