// Package jailer handles chroot preparation, rootfs configuration, and
// Firecracker jailer process spawning for VM isolation.
package jailer

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// JailerUID is the unprivileged user ID that Firecracker runs as inside the jail.
const JailerUID = 123

// JailerGID is the group ID that Firecracker runs as inside the jail.
const JailerGID = 100

// CgroupVMMOverheadMiB is extra memory (in MiB) added to the cgroup limit
// beyond guest memory. Covers Firecracker VMM process overhead, page tables,
// and kernel slab. Without this, the OOM killer can terminate the VM under
// memory pressure.
const CgroupVMMOverheadMiB = 64

// Config holds the parameters for preparing a jailer chroot.
type Config struct {
	VMId       string
	KernelSrc  string
	RootfsSrc  string
	DiskSizeGb float64 // 0 means no expansion
	HostIP     string  // host-side IP for guest DNS resolution
	Snapshot   *SnapshotConfig
	Agent      *AgentConfig
}

// SnapshotConfig holds paths for snapshot restore.
type SnapshotConfig struct {
	SnapshotFile string
	MemFile      string
}

// AgentConfig holds the agent injection parameters.
type AgentConfig struct {
	BinaryPath string
	Token      string
	Port       int
	VMId       string
}

// CgroupConfig holds cgroup resource limits.
type CgroupConfig struct {
	CPUQuotaUs  int
	CPUPeriodUs int
	MemoryBytes int64
}

// SpawnConfig holds the parameters for spawning a jailer process.
type SpawnConfig struct {
	FirecrackerBin string
	JailerBin      string
	VMId           string
	Paths          Paths
	UID            int
	GID            int
	SeccompFilter  string
	DisableSeccomp bool
	NewPidNs       bool
	Cgroup         *CgroupConfig
	NetNS          string
}

// Prepare creates the jailer chroot directory structure, links the kernel,
// copies the rootfs, optionally expands the disk, mounts the rootfs to
// configure DNS/hostname/agent, and copies snapshot files if provided.
func Prepare(cfg Config, paths Paths) error {
	// 1. Create directories
	for _, dir := range []string{paths.KernelDir, paths.RootfsDir, paths.SocketDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
	}

	// 2. Hard-link kernel (read-only, shared across VMs)
	if _, err := os.Stat(paths.KernelPath); os.IsNotExist(err) {
		if err := os.Link(cfg.KernelSrc, paths.KernelPath); err != nil {
			return fmt.Errorf("hard-link kernel: %w", err)
		}
	}

	// 3. Copy rootfs (writable per-VM)
	if err := copyFile(cfg.RootfsSrc, paths.RootfsPath); err != nil {
		return fmt.Errorf("copy rootfs: %w", err)
	}

	// 4. Expand disk if requested
	if cfg.DiskSizeGb > 0 {
		if err := expandDisk(paths.RootfsPath, cfg.DiskSizeGb); err != nil {
			return fmt.Errorf("expand disk: %w", err)
		}
	}

	// 5. Mount rootfs and configure
	if err := configureRootfs(cfg, paths); err != nil {
		return fmt.Errorf("configure rootfs: %w", err)
	}

	// 5b. Chown the entire chroot tree to the jailer UID:GID so Firecracker
	// (running as uid 123) can access all files. The jailer binary also does
	// this, but only for dirs it creates — our pre-placed files need it too.
	chownCmd := exec.Command("chown", "-R", fmt.Sprintf("%d:%d", JailerUID, JailerGID), paths.RootDir)
	if output, err := chownCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("chown chroot to jailer uid: %w: %s", err, string(output))
	}

	// 6. Copy snapshot files if restoring
	if cfg.Snapshot != nil {
		if err := os.MkdirAll(paths.SnapshotDir, 0755); err != nil {
			return fmt.Errorf("create snapshot dir: %w", err)
		}
		if err := copyFile(cfg.Snapshot.SnapshotFile, filepath.Join(paths.SnapshotDir, "snapshot_file")); err != nil {
			return fmt.Errorf("copy snapshot file: %w", err)
		}
		if err := copyFile(cfg.Snapshot.MemFile, filepath.Join(paths.SnapshotDir, "mem_file")); err != nil {
			return fmt.Errorf("copy mem file: %w", err)
		}
	}

	return nil
}

// Spawn starts the Firecracker jailer process with the given configuration.
func Spawn(cfg SpawnConfig) error {
	args := []string{
		cfg.JailerBin,
		"--exec-file", cfg.FirecrackerBin,
		"--id", cfg.VMId,
		"--uid", strconv.Itoa(cfg.UID),
		"--gid", strconv.Itoa(cfg.GID),
		"--chroot-base-dir", cfg.Paths.ChrootBase,
		"--daemonize",
	}

	if cfg.NewPidNs {
		args = append(args, "--new-pid-ns")
	}

	if cfg.NetNS != "" {
		args = append(args, "--netns", "/var/run/netns/"+cfg.NetNS)
	}

	if cfg.Cgroup != nil {
		cgroupVer := detectCgroupVersion()
		if cgroupVer == 2 {
			args = append(args, "--cgroup-version", "2")
			args = append(args, "--cgroup", fmt.Sprintf("cpu.max=%d %d", cfg.Cgroup.CPUQuotaUs, cfg.Cgroup.CPUPeriodUs))
			args = append(args, "--cgroup", fmt.Sprintf("memory.max=%d", cfg.Cgroup.MemoryBytes))
		} else {
			args = append(args, "--cgroup", fmt.Sprintf("cpu.cfs_quota_us=%d", cfg.Cgroup.CPUQuotaUs))
			args = append(args, "--cgroup", fmt.Sprintf("cpu.cfs_period_us=%d", cfg.Cgroup.CPUPeriodUs))
			args = append(args, "--cgroup", fmt.Sprintf("memory.limit_in_bytes=%d", cfg.Cgroup.MemoryBytes))
		}
	}

	// Firecracker args go after "--". The jailer passes them through to Firecracker.
	fcArgs := []string{"--api-sock", "run/firecracker.socket"}

	// Seccomp is handled by Firecracker (not the jailer) since v1.5+.
	// When neither flag is passed, Firecracker uses its built-in default filter.
	if cfg.DisableSeccomp {
		fcArgs = append(fcArgs, "--no-seccomp")
	} else if cfg.SeccompFilter != "" {
		if _, err := os.Stat(cfg.SeccompFilter); err == nil {
			fcArgs = append(fcArgs, "--seccomp-filter", cfg.SeccompFilter)
		}
		// If filter path is invalid, fall through to Firecracker's built-in default.
	}

	args = append(args, "--")
	args = append(args, fcArgs...)

	cmd := exec.Command(args[0], args[1:]...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("spawn jailer: %w: %s", err, string(output))
	}
	return nil
}

// Cleanup removes the entire chroot directory for a VM.
func Cleanup(chrootDir string) error {
	return os.RemoveAll(chrootDir)
}

// detectCgroupVersion returns 2 if cgroup v2 is available, 1 otherwise.
func detectCgroupVersion() int {
	if _, err := os.ReadFile("/sys/fs/cgroup/cgroup.controllers"); err == nil {
		return 2
	}
	return 1
}

// expandDisk grows the rootfs image to the target size if it is currently smaller.
func expandDisk(rootfsPath string, diskSizeGb float64) error {
	targetBytes := int64(diskSizeGb * 1024 * 1024 * 1024)

	info, err := os.Stat(rootfsPath)
	if err != nil {
		return fmt.Errorf("stat rootfs: %w", err)
	}
	if targetBytes <= info.Size() {
		return nil
	}

	// truncate to target size
	cmd := exec.Command("truncate", "-s", strconv.FormatInt(targetBytes, 10), rootfsPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("truncate: %w: %s", err, string(output))
	}

	// e2fsck -fy: force check + auto-fix before resize.
	e2fsck := exec.Command("e2fsck", "-fy", rootfsPath)
	e2fsckOut, e2fsckErr := e2fsck.CombinedOutput()
	e2fsckExit := -1
	if e2fsck.ProcessState != nil {
		e2fsckExit = e2fsck.ProcessState.ExitCode()
	}
	slog.Debug("e2fsck completed", "exit", e2fsckExit, "output", string(e2fsckOut), "err", e2fsckErr)
	if e2fsckExit >= 4 {
		return fmt.Errorf("e2fsck failed (exit %d): %s", e2fsckExit, string(e2fsckOut))
	}

	// resize2fs
	cmd = exec.Command("resize2fs", rootfsPath)
	resizeOut, resizeErr := cmd.CombinedOutput()
	slog.Debug("resize2fs completed", "output", string(resizeOut), "err", resizeErr)
	if resizeErr != nil {
		return fmt.Errorf("resize2fs: %w: %s", resizeErr, string(resizeOut))
	}

	// tune2fs: set reserved blocks to 0
	cmd = exec.Command("tune2fs", "-m", "0", rootfsPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tune2fs: %w: %s", err, string(output))
	}

	return nil
}

// configureRootfs mounts the rootfs, configures DNS, hostname, agent, and
// fixes permissions, then unmounts.
func configureRootfs(cfg Config, paths Paths) error {
	tmpMount := filepath.Join(paths.RootDir, "tmp-mount")
	if err := os.MkdirAll(tmpMount, 0755); err != nil {
		return fmt.Errorf("create tmp-mount: %w", err)
	}

	// Mount
	slog.Debug("mounting rootfs", "src", paths.RootfsPath, "dst", tmpMount)
	cmd := exec.Command("mount", "-o", "loop", paths.RootfsPath, tmpMount)
	if output, err := cmd.CombinedOutput(); err != nil {
		os.RemoveAll(tmpMount)
		return fmt.Errorf("mount rootfs: %w: %s", err, string(output))
	}

	// Verify mount actually worked — stale loop mounts from previous failed
	// attempts can cause mount to return exit 0 but show an empty filesystem.
	etcDir := filepath.Join(tmpMount, "etc")
	if _, err := os.Stat(etcDir); err != nil {
		entries, _ := os.ReadDir(tmpMount)
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		// Check for stale loop devices on this file
		losetup, _ := exec.Command("losetup", "-j", paths.RootfsPath).CombinedOutput()
		exec.Command("umount", "-l", tmpMount).Run()
		os.RemoveAll(tmpMount)
		return fmt.Errorf("mount rootfs: mounted but /etc missing (contents: %v, losetup: %s) — likely stale loop device from previous attempt", names, strings.TrimSpace(string(losetup)))
	}

	var prepareErr error
	func() {
		defer func() {
			// Always unmount — try normal first, then lazy as fallback.
			// Stale mounts from failed attempts can block future VM creation.
			if err := exec.Command("umount", tmpMount).Run(); err != nil {
				slog.Warn("umount failed, trying lazy unmount", "path", tmpMount, "error", err)
				exec.Command("umount", "-l", tmpMount).Run()
			}
			os.RemoveAll(tmpMount)
		}()

		// Configure DNS: write static resolv.conf pointing to host gateway.
		// Remove any symlink (Ubuntu uses resolv.conf → /run/systemd/resolve/stub-resolv.conf),
		// write a static file, then make it immutable so systemd-resolved/cloud-init can't
		// overwrite it during boot.
		resolvConf := filepath.Join(tmpMount, "etc", "resolv.conf")
		exec.Command("chattr", "-i", resolvConf).Run() // clear immutable if set from previous run
		os.Remove(resolvConf)
		nameserver := fmt.Sprintf("nameserver %s\n", cfg.HostIP)
		if err := os.WriteFile(resolvConf, []byte(nameserver), 0644); err != nil {
			prepareErr = fmt.Errorf("write resolv.conf: %w", err)
			return
		}
		exec.Command("chattr", "+i", resolvConf).Run() // make immutable

		// Set hostname
		hostnamePath := filepath.Join(tmpMount, "etc", "hostname")
		if err := os.WriteFile(hostnamePath, []byte(cfg.VMId+"\n"), 0644); err != nil {
			prepareErr = fmt.Errorf("write hostname: %w", err)
			return
		}

		// Write /etc/hosts with localhost and VM hostname
		hostsPath := filepath.Join(tmpMount, "etc", "hosts")
		hostsContent, _ := os.ReadFile(hostsPath)
		hostsStr := string(hostsContent)
		if !strings.Contains(hostsStr, "127.0.0.1") {
			hostsStr = "127.0.0.1 localhost\n" + hostsStr
		}
		if !strings.Contains(hostsStr, cfg.VMId) {
			hostsStr = strings.TrimRight(hostsStr, "\n") + "\n127.0.1.1 " + cfg.VMId + "\n"
		}
		if err := os.WriteFile(hostsPath, []byte(hostsStr), 0644); err != nil {
			prepareErr = fmt.Errorf("write hosts: %w", err)
			return
		}

		// Inject agent binary and systemd service
		if cfg.Agent != nil {
			if err := injectAgent(tmpMount, cfg.Agent); err != nil {
				prepareErr = fmt.Errorf("inject agent: %w", err)
				return
			}
		}

		// Fix ubuntu home ownership
		if err := fixUbuntuOwnership(tmpMount); err != nil {
			slog.Debug("fix ubuntu ownership failed", "error", err)
		}
	}()

	return prepareErr
}

// injectAgent copies the agent binary into the rootfs and writes the
// systemd service and environment file.
func injectAgent(tmpMount string, agent *AgentConfig) error {
	// Copy agent binary
	agentBinDir := filepath.Join(tmpMount, "usr", "local", "bin")
	if err := os.MkdirAll(agentBinDir, 0755); err != nil {
		return fmt.Errorf("create agent bin dir: %w", err)
	}
	agentDst := filepath.Join(agentBinDir, "vmsan-agent")
	if err := copyFile(agent.BinaryPath, agentDst); err != nil {
		return fmt.Errorf("copy agent binary: %w", err)
	}
	if err := os.Chmod(agentDst, 0755); err != nil {
		return fmt.Errorf("chmod agent binary: %w", err)
	}

	// Write agent env
	envDir := filepath.Join(tmpMount, "etc", "vmsan")
	if err := os.MkdirAll(envDir, 0755); err != nil {
		return fmt.Errorf("create env dir: %w", err)
	}
	envContent := fmt.Sprintf(
		"VMSAN_AGENT_TOKEN=%s\nVMSAN_AGENT_PORT=%d\nVMSAN_VM_ID=%s\nVMSAN_DEFAULT_USER=ubuntu\n",
		agent.Token, agent.Port, agent.VMId,
	)
	if err := os.WriteFile(filepath.Join(envDir, "agent.env"), []byte(envContent), 0644); err != nil {
		return fmt.Errorf("write agent env: %w", err)
	}

	// Write systemd service
	systemdDir := filepath.Join(tmpMount, "etc", "systemd", "system")
	if err := os.MkdirAll(systemdDir, 0755); err != nil {
		return fmt.Errorf("create systemd dir: %w", err)
	}
	serviceContent := `[Unit]
Description=Vmsan VM Agent
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/vmsan-agent
EnvironmentFile=/etc/vmsan/agent.env
Restart=always
RestartSec=2

[Install]
WantedBy=multi-user.target
`
	servicePath := filepath.Join(systemdDir, "vmsan-agent.service")
	if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		return fmt.Errorf("write agent service: %w", err)
	}

	// Create symlink for auto-start
	wantsDir := filepath.Join(systemdDir, "multi-user.target.wants")
	if err := os.MkdirAll(wantsDir, 0755); err != nil {
		return fmt.Errorf("create wants dir: %w", err)
	}
	symlinkPath := filepath.Join(wantsDir, "vmsan-agent.service")
	os.Remove(symlinkPath) // remove stale symlink if present
	if err := os.Symlink("/etc/systemd/system/vmsan-agent.service", symlinkPath); err != nil {
		return fmt.Errorf("symlink agent service: %w", err)
	}

	return nil
}

// fixUbuntuOwnership reads the rootfs /etc/passwd to find the ubuntu user's
// uid:gid and chowns /home/ubuntu recursively.
func fixUbuntuOwnership(tmpMount string) error {
	ubuntuHome := filepath.Join(tmpMount, "home", "ubuntu")
	if _, err := os.Stat(ubuntuHome); os.IsNotExist(err) {
		return nil
	}

	passwdPath := filepath.Join(tmpMount, "etc", "passwd")
	f, err := os.Open(passwdPath)
	if err != nil {
		return nil // no passwd file, nothing to fix
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "ubuntu:") {
			continue
		}
		fields := strings.Split(line, ":")
		if len(fields) < 4 {
			return nil
		}
		uid, err1 := strconv.Atoi(fields[2])
		gid, err2 := strconv.Atoi(fields[3])
		if err1 != nil || err2 != nil {
			return nil
		}
		cmd := exec.Command("chown", "-R", fmt.Sprintf("%d:%d", uid, gid), ubuntuHome)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("chown ubuntu home: %w: %s", err, string(output))
		}
		return nil
	}

	return nil
}

// copyFile copies src to dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
