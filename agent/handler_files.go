package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const maxTarUploadBytes = 1 << 30 // 1GB

func handleFilesWrite(w http.ResponseWriter, r *http.Request) {
	extractDir := r.Header.Get("X-Extract-Dir")
	if extractDir == "" {
		extractDir = "/"
	}

	// Ensure extract dir is absolute and clean.
	extractDir = filepath.Clean(extractDir)
	if !filepath.IsAbs(extractDir) {
		http.Error(w, `{"error":"X-Extract-Dir must be absolute"}`, http.StatusBadRequest)
		return
	}

	lr := &io.LimitedReader{R: r.Body, N: maxTarUploadBytes + 1}
	gz, err := gzip.NewReader(lr)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"gzip: %s"}`, err), http.StatusBadRequest)
		return
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	filesWritten := 0

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"tar: %s"}`, err), http.StatusBadRequest)
			return
		}

		// Path traversal protection.
		target := filepath.Join(extractDir, header.Name)
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(extractDir)+string(os.PathSeparator)) &&
			filepath.Clean(target) != filepath.Clean(extractDir) {
			http.Error(w, `{"error":"path traversal detected"}`, http.StatusBadRequest)
			return
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"mkdir: %s"}`, err), http.StatusInternalServerError)
				return
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"mkdir: %s"}`, err), http.StatusInternalServerError)
				return
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"create: %s"}`, err), http.StatusInternalServerError)
				return
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				http.Error(w, fmt.Sprintf(`{"error":"write: %s"}`, err), http.StatusInternalServerError)
				return
			}
			f.Close()
			filesWritten++
		}

		if lr.N <= 0 {
			http.Error(w, `{"error":"upload exceeds 1GB limit"}`, http.StatusRequestEntityTooLarge)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"filesWritten": filesWritten,
	})
}

type readRequest struct {
	Path string `json:"path"`
}

func handleFilesRead(w http.ResponseWriter, r *http.Request) {
	var req readRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if req.Path == "" {
		http.Error(w, `{"error":"path is required"}`, http.StatusBadRequest)
		return
	}

	// Ensure absolute path.
	cleanPath := filepath.Clean(req.Path)
	if !filepath.IsAbs(cleanPath) {
		http.Error(w, `{"error":"path must be absolute"}`, http.StatusBadRequest)
		return
	}

	info, err := os.Stat(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, `{"error":"file not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, fmt.Sprintf(`{"error":"stat: %s"}`, err), http.StatusInternalServerError)
		return
	}
	if info.IsDir() {
		http.Error(w, `{"error":"path is a directory"}`, http.StatusBadRequest)
		return
	}

	f, err := os.Open(cleanPath)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"open: %s"}`, err), http.StatusInternalServerError)
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))
	io.Copy(w, f)
}
