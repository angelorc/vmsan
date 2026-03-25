package paths

import (
	"os"
	"os/user"
	"path/filepath"
)

// Paths holds all resolved vmsan directory and binary paths.
type Paths struct {
	BaseDir       string
	VmsDir        string
	JailerBaseDir string
	BinDir        string
	AgentBin      string
	NftablesBin   string // kept for compat, no longer used
	GatewayBin    string
	DnsproxyBin   string
	KernelsDir    string
	RootfsDir     string
	RegistryDir   string
	SnapshotsDir  string
	SeccompDir    string
	SeccompFilter string
	AgentPort     int
}

// Resolve returns paths for the current user/environment.
func Resolve() Paths {
	base := resolveBaseDir()
	binDir := filepath.Join(base, "bin")
	return Paths{
		BaseDir:       base,
		VmsDir:        filepath.Join(base, "vms"),
		JailerBaseDir: filepath.Join(base, "jailer"),
		BinDir:        binDir,
		AgentBin:      resolveBin(base, "vmsan-agent"),
		GatewayBin:    resolveBin(base, "vmsan-gateway"),
		DnsproxyBin:   resolveBin(base, "dnsproxy"),
		KernelsDir:    filepath.Join(base, "kernels"),
		RootfsDir:     filepath.Join(base, "rootfs"),
		RegistryDir:   filepath.Join(base, "registry", "rootfs"),
		SnapshotsDir:  filepath.Join(base, "snapshots"),
		SeccompDir:    filepath.Join(base, "seccomp"),
		SeccompFilter: resolveSeccompFilter(base),
		AgentPort:     9119,
	}
}

// resolveSeccompFilter returns the path to a compiled seccomp BPF filter.
// Firecracker's --seccomp-filter flag requires a pre-compiled BPF file,
// not raw JSON. The install script compiles JSON → BPF using seccompiler-bin.
//
// Preference order:
//  1. firecracker-default.bpf — official Firecracker filter (most compatible)
//  2. default.bpf — vmsan custom filter
//
// Returns empty string if no compiled filter found — Firecracker will use
// its built-in default filter (may be too restrictive on newer kernels).
func resolveSeccompFilter(base string) string {
	seccompDir := filepath.Join(base, "seccomp")
	for _, name := range []string{"firecracker-default.bpf", "default.bpf"} {
		bpf := filepath.Join(seccompDir, name)
		if _, err := os.Stat(bpf); err == nil {
			return bpf
		}
	}
	return ""
}

func resolveBaseDir() string {
	if dir := os.Getenv("VMSAN_DIR"); dir != "" {
		return dir
	}
	// When running as root via sudo, use the original user's home
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		if u, err := user.Lookup(sudoUser); err == nil {
			return filepath.Join(u.HomeDir, ".vmsan")
		}
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".vmsan")
}

// resolveBin prefers ~/.vmsan/bin/<name>, falls back to /usr/local/bin/<name>.
func resolveBin(base, name string) string {
	userPath := filepath.Join(base, "bin", name)
	if _, err := os.Stat(userPath); err == nil {
		return userPath
	}
	systemPath := filepath.Join("/usr/local/bin", name)
	if _, err := os.Stat(systemPath); err == nil {
		return systemPath
	}
	return userPath // return user path even if missing (for error messages)
}
