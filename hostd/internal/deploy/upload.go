package deploy

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/angelorc/vmsan/hostd/internal/agentclient"
)

const (
	maxBatchBytes = 50 * 1024 * 1024 // 50 MB per batch
	destDir       = "/app"
)

// UploadResult holds the outcome of uploading source files.
type UploadResult struct {
	FilesUploaded int   `json:"filesUploaded"`
	BytesUploaded int64 `json:"bytesUploaded"`
}

// UploadSource recursively uploads source files to the VM agent.
// Respects .gitignore and .vmsanignore patterns.
func UploadSource(ctx context.Context, agent *agentclient.Client, sourceDir string) error {
	ignorePatterns := loadIgnorePatterns(sourceDir)

	var batch []agentclient.WriteFileEntry
	var batchSize int64

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := agent.WriteFiles(ctx, batch, destDir); err != nil {
			return err
		}
		batch = batch[:0]
		batchSize = 0
		return nil
	}

	err := filepath.WalkDir(sourceDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, _ := filepath.Rel(sourceDir, path)
		if rel == "." {
			return nil
		}

		// Always skip these directories
		name := d.Name()
		if d.IsDir() {
			if name == ".git" || name == "node_modules" || name == ".vmsan" {
				return filepath.SkipDir
			}
			if shouldIgnore(rel, true, ignorePatterns) {
				return filepath.SkipDir
			}
			return nil
		}

		if shouldIgnore(rel, false, ignorePatterns) {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil // skip unreadable files
		}

		info, _ := d.Info()
		mode := int64(0644)
		if info != nil && info.Mode()&0111 != 0 {
			mode = 0755
		}

		fileSize := int64(len(data))
		if batchSize+fileSize > maxBatchBytes {
			if err := flush(); err != nil {
				return err
			}
		}

		batch = append(batch, agentclient.WriteFileEntry{
			Path:    rel,
			Content: data,
			Mode:    mode,
		})
		batchSize += fileSize
		return nil
	})

	if err != nil {
		return fmt.Errorf("walk source dir: %w", err)
	}

	return flush()
}

// --- ignore pattern handling ---

func loadIgnorePatterns(dir string) []string {
	var patterns []string
	for _, name := range []string{".gitignore", ".vmsanignore"} {
		path := filepath.Join(dir, name)
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			patterns = append(patterns, line)
		}
		f.Close()
	}
	return patterns
}

func shouldIgnore(relPath string, isDir bool, patterns []string) bool {
	for _, pattern := range patterns {
		// Directory-only pattern (trailing /)
		if strings.HasSuffix(pattern, "/") {
			if isDir && matchGlob(relPath, strings.TrimSuffix(pattern, "/")) {
				return true
			}
			continue
		}
		if matchGlob(relPath, pattern) {
			return true
		}
		// Also match against basename
		base := filepath.Base(relPath)
		if matchGlob(base, pattern) {
			return true
		}
	}
	return false
}

// matchGlob provides simple glob matching supporting * and **.
func matchGlob(path, pattern string) bool {
	// Handle ** (match any path segment)
	if strings.Contains(pattern, "**") {
		parts := strings.SplitN(pattern, "**", 2)
		prefix := strings.TrimSuffix(parts[0], "/")
		suffix := strings.TrimPrefix(parts[1], "/")

		if prefix != "" && !strings.HasPrefix(path, prefix) {
			return false
		}
		if suffix == "" {
			return true
		}
		// Check if any suffix of path matches
		segments := strings.Split(path, "/")
		for i := range segments {
			remainder := strings.Join(segments[i:], "/")
			if matchSimple(remainder, suffix) {
				return true
			}
		}
		return false
	}
	return matchSimple(path, pattern)
}

// matchSimple handles single * wildcards.
func matchSimple(path, pattern string) bool {
	if !strings.Contains(pattern, "*") {
		return path == pattern
	}

	parts := strings.Split(pattern, "*")
	if len(parts) == 2 {
		return strings.HasPrefix(path, parts[0]) && strings.HasSuffix(path, parts[1])
	}

	// Multi-star: check prefix, suffix, and intermediate parts
	if !strings.HasPrefix(path, parts[0]) {
		return false
	}
	if !strings.HasSuffix(path, parts[len(parts)-1]) {
		return false
	}

	remaining := path[len(parts[0]):]
	for i := 1; i < len(parts)-1; i++ {
		idx := strings.Index(remaining, parts[i])
		if idx == -1 {
			return false
		}
		remaining = remaining[idx+len(parts[i]):]
	}
	return true
}
