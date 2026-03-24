package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectProject_Node(t *testing.T) {
	dir := t.TempDir()
	pkgJSON := `{"scripts":{"start":"node server.js","build":"npm run build"}}`
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkgJSON), 0644); err != nil {
		t.Fatal(err)
	}

	r := DetectProject(dir)
	if r == nil {
		t.Fatal("DetectProject() returned nil, want non-nil")
	}
	if r.Runtime != "node22" {
		t.Errorf("Runtime = %q, want %q", r.Runtime, "node22")
	}
	if r.Build != "npm run build" {
		t.Errorf("Build = %q, want %q", r.Build, "npm run build")
	}
	if r.Start != "npm start" {
		t.Errorf("Start = %q, want %q", r.Start, "npm start")
	}
}

func TestDetectProject_Go(t *testing.T) {
	dir := t.TempDir()
	goMod := "module example.com/myapp\n\ngo 1.23\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatal(err)
	}

	r := DetectProject(dir)
	if r == nil {
		t.Fatal("DetectProject() returned nil, want non-nil")
	}
	if r.Runtime != "base" {
		t.Errorf("Runtime = %q, want %q", r.Runtime, "base")
	}
	if r.Build != "go build -o app ." {
		t.Errorf("Build = %q, want %q", r.Build, "go build -o app .")
	}
	if r.Start != "./app" {
		t.Errorf("Start = %q, want %q", r.Start, "./app")
	}
}

func TestDetectProject_Python_FastAPI(t *testing.T) {
	dir := t.TempDir()
	reqs := "fastapi>=0.100.0\nuvicorn\npydantic\n"
	if err := os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte(reqs), 0644); err != nil {
		t.Fatal(err)
	}

	r := DetectProject(dir)
	if r == nil {
		t.Fatal("DetectProject() returned nil, want non-nil")
	}
	if r.Runtime != "python3.13" {
		t.Errorf("Runtime = %q, want %q", r.Runtime, "python3.13")
	}
	if !strings.Contains(r.Start, "uvicorn") {
		t.Errorf("Start = %q, want it to contain 'uvicorn'", r.Start)
	}
}

func TestDetectProject_Rust(t *testing.T) {
	dir := t.TempDir()
	cargoToml := `[package]
name = "myapp"
version = "0.1.0"
edition = "2021"
`
	if err := os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte(cargoToml), 0644); err != nil {
		t.Fatal(err)
	}

	r := DetectProject(dir)
	if r == nil {
		t.Fatal("DetectProject() returned nil, want non-nil")
	}
	if r.Runtime != "base" {
		t.Errorf("Runtime = %q, want %q", r.Runtime, "base")
	}
	if r.Start != "./myapp" {
		t.Errorf("Start = %q, want %q", r.Start, "./myapp")
	}
}

func TestDetectProject_Empty(t *testing.T) {
	dir := t.TempDir()
	r := DetectProject(dir)
	if r != nil {
		t.Errorf("DetectProject() = %+v, want nil for empty dir", r)
	}
}
