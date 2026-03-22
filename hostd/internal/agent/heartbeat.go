package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// resources holds the current host resource snapshot.
type resources struct {
	CPUs     int   `json:"cpus"`
	MemoryMB int64 `json:"memory_mb"`
	DiskGB   int64 `json:"disk_gb"`
}

// heartbeatPayload is the body sent with each heartbeat.
type heartbeatPayload struct {
	Resources resources `json:"resources"`
}

// heartbeatLoop sends periodic heartbeats to the server.
func (a *Agent) heartbeatLoop(ctx context.Context) {
	// Send one heartbeat immediately on startup.
	a.sendHeartbeat()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.sendHeartbeat()
		}
	}
}

// sendHeartbeat collects host resources and sends them to the server.
func (a *Agent) sendHeartbeat() {
	res := detectResources()

	body, err := json.Marshal(heartbeatPayload{Resources: res})
	if err != nil {
		a.logger.Warn("failed to marshal heartbeat", "error", err)
		return
	}

	url := fmt.Sprintf("%s/api/v1/hosts/%s/heartbeat", a.config.ServerURL, a.config.HostID)

	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		a.logger.Warn("failed to create heartbeat request", "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.http.Do(req)
	if err != nil {
		a.logger.Warn("heartbeat failed", "error", err)
		return
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		a.logger.Warn("heartbeat rejected", "status", resp.StatusCode)
		return
	}

	a.logger.Debug("heartbeat sent", "cpus", res.CPUs, "memory_mb", res.MemoryMB, "disk_gb", res.DiskGB)
}

// detectResources gathers CPU, memory, and disk information from the host.
func detectResources() resources {
	return resources{
		CPUs:     runtime.NumCPU(),
		MemoryMB: detectMemoryMB(),
		DiskGB:   detectDiskGB(),
	}
}

// detectMemoryMB reads total memory from /proc/meminfo.
func detectMemoryMB() int64 {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}

	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				kb, err := strconv.ParseInt(fields[1], 10, 64)
				if err == nil {
					return kb / 1024
				}
			}
			break
		}
	}

	return 0
}

// detectDiskGB returns available disk space on the root filesystem in GB.
func detectDiskGB() int64 {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err != nil {
		return 0
	}
	// Available blocks * block size → bytes → GB.
	return int64(stat.Bavail) * int64(stat.Bsize) / (1024 * 1024 * 1024)
}
