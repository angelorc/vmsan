package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// DetectionResult holds the auto-detected project configuration.
type DetectionResult struct {
	Runtime    string `json:"runtime"`
	Build      string `json:"build,omitempty"`
	Start      string `json:"start,omitempty"`
	Confidence string `json:"confidence"` // "high", "medium", "low"
	Reason     string `json:"reason"`
}

// DetectProject auto-detects the project runtime, build command, and start command.
func DetectProject(dir string) *DetectionResult {
	if r := detectNode(dir); r != nil {
		return r
	}
	if r := detectGo(dir); r != nil {
		return r
	}
	if r := detectPython(dir); r != nil {
		return r
	}
	if r := detectRust(dir); r != nil {
		return r
	}
	if r := detectDocker(dir); r != nil {
		return r
	}
	return nil
}

func detectNode(dir string) *DetectionResult {
	pkgPath := filepath.Join(dir, "package.json")
	data, err := os.ReadFile(pkgPath)
	if err != nil {
		return nil
	}

	var pkg struct {
		Scripts struct {
			Start string `json:"start"`
			Build string `json:"build"`
		} `json:"scripts"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil
	}

	// Detect Node version from .node-version or .nvmrc
	runtime := "node22" // default
	for _, versionFile := range []string{".node-version", ".nvmrc"} {
		if data, err := os.ReadFile(filepath.Join(dir, versionFile)); err == nil {
			version := strings.TrimSpace(string(data))
			if strings.HasPrefix(version, "24") || strings.HasPrefix(version, "v24") {
				runtime = "node24"
			}
		}
	}

	start := pkg.Scripts.Start
	if start == "" {
		// Common patterns
		if fileExists(filepath.Join(dir, "server.js")) {
			start = "node server.js"
		} else if fileExists(filepath.Join(dir, "index.js")) {
			start = "node index.js"
		} else if fileExists(filepath.Join(dir, "app.js")) {
			start = "node app.js"
		}
	} else {
		start = "npm start"
	}

	build := ""
	if pkg.Scripts.Build != "" {
		build = "npm run build"
	}

	confidence := "high"
	if start == "" {
		confidence = "medium"
	}

	return &DetectionResult{
		Runtime:    runtime,
		Build:      build,
		Start:      start,
		Confidence: confidence,
		Reason:     "Detected package.json",
	}
}

func detectGo(dir string) *DetectionResult {
	if !fileExists(filepath.Join(dir, "go.mod")) {
		return nil
	}
	return &DetectionResult{
		Runtime:    "base",
		Build:      "go build -o app .",
		Start:      "./app",
		Confidence: "high",
		Reason:     "Detected go.mod",
	}
}

func detectPython(dir string) *DetectionResult {
	hasRequirements := fileExists(filepath.Join(dir, "requirements.txt"))
	hasPyproject := fileExists(filepath.Join(dir, "pyproject.toml"))
	if !hasRequirements && !hasPyproject {
		return nil
	}

	result := &DetectionResult{
		Runtime:    "python3.13",
		Confidence: "medium",
		Reason:     "Detected Python project",
	}

	if hasRequirements {
		result.Build = "pip install -r requirements.txt"
	} else {
		result.Build = "pip install ."
	}

	// Detect framework
	if fileExists(filepath.Join(dir, "manage.py")) {
		result.Start = "python manage.py runserver 0.0.0.0:8000"
		result.Confidence = "high"
		result.Reason = "Detected Django project"
	} else if containsInFile(filepath.Join(dir, "requirements.txt"), "fastapi") {
		result.Start = "uvicorn main:app --host 0.0.0.0 --port 8000"
		result.Reason = "Detected FastAPI project"
	} else if containsInFile(filepath.Join(dir, "requirements.txt"), "flask") {
		result.Start = "flask run --host 0.0.0.0 --port 8000"
		result.Reason = "Detected Flask project"
	}

	return result
}

func detectRust(dir string) *DetectionResult {
	cargoPath := filepath.Join(dir, "Cargo.toml")
	if !fileExists(cargoPath) {
		return nil
	}

	// Try to extract binary name from Cargo.toml
	start := "./app"
	if data, err := os.ReadFile(cargoPath); err == nil {
		content := string(data)
		// Simple extraction: look for name = "..."
		if idx := strings.Index(content, "name = \""); idx != -1 {
			rest := content[idx+8:]
			if end := strings.Index(rest, "\""); end != -1 {
				start = "./" + rest[:end]
			}
		}
	}

	return &DetectionResult{
		Runtime:    "base",
		Build:      "cargo build --release && cp target/release/* ./app 2>/dev/null || true",
		Start:      start,
		Confidence: "high",
		Reason:     "Detected Cargo.toml",
	}
}

func detectDocker(dir string) *DetectionResult {
	if !fileExists(filepath.Join(dir, "Dockerfile")) {
		return nil
	}
	return &DetectionResult{
		Runtime:    "base",
		Confidence: "low",
		Reason:     "Detected Dockerfile (manual configuration recommended)",
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func containsInFile(path, substr string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(data)), strings.ToLower(substr))
}
