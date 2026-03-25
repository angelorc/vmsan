package paths

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolve_Default(t *testing.T) {
	// Clear env vars that could affect resolution.
	t.Setenv("VMSAN_DIR", "")
	t.Setenv("SUDO_USER", "")

	p := Resolve()

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}

	wantBase := filepath.Join(home, ".vmsan")
	if p.BaseDir != wantBase {
		t.Errorf("BaseDir = %q, want %q", p.BaseDir, wantBase)
	}

	wantVms := filepath.Join(wantBase, "vms")
	if p.VmsDir != wantVms {
		t.Errorf("VmsDir = %q, want %q", p.VmsDir, wantVms)
	}

	wantJailer := filepath.Join(wantBase, "jailer")
	if p.JailerBaseDir != wantJailer {
		t.Errorf("JailerBaseDir = %q, want %q", p.JailerBaseDir, wantJailer)
	}

	wantBin := filepath.Join(wantBase, "bin")
	if p.BinDir != wantBin {
		t.Errorf("BinDir = %q, want %q", p.BinDir, wantBin)
	}

	wantKernels := filepath.Join(wantBase, "kernels")
	if p.KernelsDir != wantKernels {
		t.Errorf("KernelsDir = %q, want %q", p.KernelsDir, wantKernels)
	}

	wantRootfs := filepath.Join(wantBase, "rootfs")
	if p.RootfsDir != wantRootfs {
		t.Errorf("RootfsDir = %q, want %q", p.RootfsDir, wantRootfs)
	}

	wantRegistry := filepath.Join(wantBase, "registry", "rootfs")
	if p.RegistryDir != wantRegistry {
		t.Errorf("RegistryDir = %q, want %q", p.RegistryDir, wantRegistry)
	}

	wantSnapshots := filepath.Join(wantBase, "snapshots")
	if p.SnapshotsDir != wantSnapshots {
		t.Errorf("SnapshotsDir = %q, want %q", p.SnapshotsDir, wantSnapshots)
	}

	wantSeccomp := filepath.Join(wantBase, "seccomp")
	if p.SeccompDir != wantSeccomp {
		t.Errorf("SeccompDir = %q, want %q", p.SeccompDir, wantSeccomp)
	}

	// SeccompFilter resolves to the first existing .bpf file, or empty string.
	if p.SeccompFilter != "" && !strings.HasSuffix(p.SeccompFilter, ".bpf") {
		t.Errorf("SeccompFilter = %q, want .bpf suffix or empty", p.SeccompFilter)
	}

	if !strings.HasSuffix(p.BaseDir, ".vmsan") {
		t.Errorf("BaseDir should end with .vmsan, got %q", p.BaseDir)
	}
}

func TestResolve_CustomDir(t *testing.T) {
	t.Setenv("VMSAN_DIR", "/custom/path")

	p := Resolve()

	if p.BaseDir != "/custom/path" {
		t.Errorf("BaseDir = %q, want %q", p.BaseDir, "/custom/path")
	}
	if p.VmsDir != "/custom/path/vms" {
		t.Errorf("VmsDir = %q, want %q", p.VmsDir, "/custom/path/vms")
	}
	if p.BinDir != "/custom/path/bin" {
		t.Errorf("BinDir = %q, want %q", p.BinDir, "/custom/path/bin")
	}
	if p.KernelsDir != "/custom/path/kernels" {
		t.Errorf("KernelsDir = %q, want %q", p.KernelsDir, "/custom/path/kernels")
	}
	if p.RootfsDir != "/custom/path/rootfs" {
		t.Errorf("RootfsDir = %q, want %q", p.RootfsDir, "/custom/path/rootfs")
	}
}

func TestResolve_AgentPort(t *testing.T) {
	p := Resolve()
	if p.AgentPort != 9119 {
		t.Errorf("AgentPort = %d, want 9119", p.AgentPort)
	}
}

func TestResolveBaseDir_EnvOverride(t *testing.T) {
	t.Setenv("VMSAN_DIR", "/override/dir")
	t.Setenv("SUDO_USER", "")

	got := resolveBaseDir()
	if got != "/override/dir" {
		t.Errorf("resolveBaseDir() = %q, want %q", got, "/override/dir")
	}
}

func TestResolveBaseDir_DefaultFallback(t *testing.T) {
	t.Setenv("VMSAN_DIR", "")
	t.Setenv("SUDO_USER", "")

	got := resolveBaseDir()
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".vmsan")
	if got != want {
		t.Errorf("resolveBaseDir() = %q, want %q", got, want)
	}
}

func TestResolveBin_FallbackToUserPath(t *testing.T) {
	// When neither user path nor system path exist, resolveBin returns user path.
	base := t.TempDir()
	got := resolveBin(base, "nonexistent-binary")
	want := filepath.Join(base, "bin", "nonexistent-binary")
	if got != want {
		t.Errorf("resolveBin() = %q, want %q", got, want)
	}
}

func TestResolveBin_UserPathExists(t *testing.T) {
	base := t.TempDir()
	binDir := filepath.Join(base, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	binPath := filepath.Join(binDir, "my-binary")
	if err := os.WriteFile(binPath, []byte("binary"), 0755); err != nil {
		t.Fatalf("write: %v", err)
	}

	got := resolveBin(base, "my-binary")
	if got != binPath {
		t.Errorf("resolveBin() = %q, want %q", got, binPath)
	}
}
