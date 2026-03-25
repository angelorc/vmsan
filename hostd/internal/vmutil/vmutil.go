package vmutil

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// AutoDetectKernel finds the first vmlinux* file in kernelsDir.
func AutoDetectKernel(kernelsDir string) string {
	entries, err := os.ReadDir(kernelsDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "vmlinux") {
			return filepath.Join(kernelsDir, e.Name())
		}
	}
	return ""
}

// AutoDetectRootfs finds the best rootfs image for the given runtime.
// Search order: exact runtime match, then "base", then "ubuntu-24.04" fallback.
func AutoDetectRootfs(rootfsDir, runtime string) string {
	candidates := []string{
		filepath.Join(rootfsDir, runtime+".ext4"),
		filepath.Join(rootfsDir, runtime+".squashfs"),
	}
	if runtime != "base" {
		candidates = append(candidates,
			filepath.Join(rootfsDir, "base.ext4"),
			filepath.Join(rootfsDir, "base.squashfs"),
		)
	}
	// Fallback to ubuntu rootfs (install.sh default name)
	candidates = append(candidates, filepath.Join(rootfsDir, "ubuntu-24.04.ext4"))
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

// GenerateToken creates a cryptographically random 64-character hex token.
func GenerateToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ParseDiskSize parses a disk size string like "10gb" and returns the size in GB.
func ParseDiskSize(value string) (int, error) {
	raw := strings.ToLower(strings.TrimSpace(value))
	re := regexp.MustCompile(`^(\d+)(gb|g|gib)?$`)
	m := re.FindStringSubmatch(raw)
	if m == nil {
		return 0, fmt.Errorf("invalid disk size format: %s (expected e.g. 10gb)", value)
	}
	size, _ := strconv.Atoi(m[1])
	if size < 1 || size > 1024 {
		return 0, fmt.Errorf("disk size must be 1-1024 GB, got %d", size)
	}
	return size, nil
}
