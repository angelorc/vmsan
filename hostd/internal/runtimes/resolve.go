// Package runtimes resolves kernel and rootfs paths for vmsan VM runtimes.
//
// This package mirrors the TypeScript resolution logic in
// src/commands/create/environment.ts and provides a single Go implementation
// used by both the agent (primary) and gateway (fallback).
package runtimes

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Known runtime-to-rootfs mappings. Must stay in sync with the TS
// RUNTIME_ROOTFS_MAP in src/commands/create/environment.ts.
var namedRootfs = map[string]string{
	"node22":     "node22.ext4",
	"node24":     "node24.ext4",
	"python3.13": "python3.13.ext4",
}

// baseRootfs are tried in order for the "base" runtime before falling back
// to any .ext4 file not claimed by a named runtime.
var baseRootfs = []string{"ubuntu-24.04.ext4"}

// Resolve locates the kernel and rootfs files for the given runtime inside
// vmsanDir (typically ~/.vmsan). It returns absolute paths.
func Resolve(vmsanDir, runtime string) (kernel, rootfs string, err error) {
	kernel, err = ResolveKernel(vmsanDir, runtime)
	if err != nil {
		return "", "", err
	}
	rootfs, err = ResolveRootfs(vmsanDir, runtime)
	if err != nil {
		return "", "", err
	}
	return kernel, rootfs, nil
}

// ResolveKernel finds the kernel binary for the given runtime.
// Search order:
//  1. runtimes/<runtime>/vmlinux*  (per-runtime override)
//  2. kernels/vmlinux*             (shared kernel)
//
// Within each directory the last file alphabetically is chosen (highest version).
func ResolveKernel(vmsanDir, runtime string) (string, error) {
	if p := findVmlinux(filepath.Join(vmsanDir, "runtimes", runtime)); p != "" {
		return p, nil
	}
	if p := findVmlinux(filepath.Join(vmsanDir, "kernels")); p != "" {
		return p, nil
	}
	return "", fmt.Errorf("no vmlinux kernel found in %s", vmsanDir)
}

// ResolveRootfs finds the rootfs image for the given runtime.
// Search order:
//  1. runtimes/<runtime>/rootfs.ext4  (bundled layout)
//  2. rootfs/<name>.ext4              (flat layout, name depends on runtime)
//
// For the "base" runtime, known base filenames are tried first, then any .ext4
// file not claimed by a named runtime.
func ResolveRootfs(vmsanDir, runtime string) (string, error) {
	// Bundled layout: runtimes/<runtime>/rootfs.ext4
	runtimeDir := filepath.Join(vmsanDir, "runtimes", runtime)
	if p := filepath.Join(runtimeDir, "rootfs.ext4"); fileExists(p) {
		return p, nil
	}

	rootfsDir := filepath.Join(vmsanDir, "rootfs")

	// Named runtime: direct mapping then <runtime>.ext4.
	if runtime != "base" {
		if filename, ok := namedRootfs[runtime]; ok {
			if p := filepath.Join(rootfsDir, filename); fileExists(p) {
				return p, nil
			}
		}
		if p := filepath.Join(rootfsDir, runtime+".ext4"); fileExists(p) {
			return p, nil
		}
		return "", fmt.Errorf("rootfs for runtime %q not found in %s", runtime, vmsanDir)
	}

	// Base runtime: try well-known filenames.
	for _, name := range baseRootfs {
		if p := filepath.Join(rootfsDir, name); fileExists(p) {
			return p, nil
		}
	}

	// Last resort: any .ext4 not claimed by a named runtime.
	claimed := make(map[string]bool, len(namedRootfs))
	for _, f := range namedRootfs {
		claimed[f] = true
	}
	entries, err := os.ReadDir(rootfsDir)
	if err != nil {
		return "", fmt.Errorf("rootfs for runtime %q not found in %s", runtime, vmsanDir)
	}
	var candidates []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".ext4") && !claimed[e.Name()] {
			candidates = append(candidates, e.Name())
		}
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("rootfs for runtime %q not found in %s", runtime, vmsanDir)
	}
	sort.Strings(candidates)
	return filepath.Join(rootfsDir, candidates[len(candidates)-1]), nil
}

// findVmlinux returns the path to the latest vmlinux* file in dir, or "".
func findVmlinux(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var matches []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "vmlinux") {
			matches = append(matches, e.Name())
		}
	}
	if len(matches) == 0 {
		return ""
	}
	sort.Strings(matches)
	return filepath.Join(dir, matches[len(matches)-1])
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
