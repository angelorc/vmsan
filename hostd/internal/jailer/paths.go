package jailer

import "path/filepath"

// Paths holds all filesystem paths for a jailer chroot.
type Paths struct {
	ChrootBase  string
	ChrootDir   string
	RootDir     string
	KernelDir   string
	KernelPath  string
	RootfsDir   string
	RootfsPath  string
	SocketDir   string
	SocketPath  string
	SnapshotDir string
}

// NewPaths computes all jailer paths from a VM ID and base directory.
func NewPaths(vmId, jailerBaseDir string) Paths {
	chrootDir := filepath.Join(jailerBaseDir, "firecracker", vmId)
	rootDir := filepath.Join(chrootDir, "root")
	return Paths{
		ChrootBase:  jailerBaseDir,
		ChrootDir:   chrootDir,
		RootDir:     rootDir,
		KernelDir:   filepath.Join(rootDir, "kernel"),
		KernelPath:  filepath.Join(rootDir, "kernel", "vmlinux"),
		RootfsDir:   filepath.Join(rootDir, "rootfs"),
		RootfsPath:  filepath.Join(rootDir, "rootfs", "rootfs.ext4"),
		SocketDir:   filepath.Join(rootDir, "run"),
		SocketPath:  filepath.Join(rootDir, "run", "firecracker.socket"),
		SnapshotDir: filepath.Join(rootDir, "snapshot"),
	}
}
