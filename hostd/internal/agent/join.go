package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// joinRequest is the payload sent to the server when joining.
type joinRequest struct {
	Token   string `json:"token"`
	Name    string `json:"name"`
	Address string `json:"address"`
}

// joinResponse is the server's reply to a successful join.
type joinResponse struct {
	HostID string `json:"host_id"`
}

// Join registers this host with the vmsan server and persists the
// resulting configuration to ~/.vmsan/agent.json.
func Join(serverURL, token, name string, logger *slog.Logger) error {
	client := &http.Client{Timeout: 30 * time.Second}

	localIP, err := detectLocalIP()
	if err != nil {
		return fmt.Errorf("detect local IP: %w", err)
	}

	logger.Info("joining server",
		"server", serverURL,
		"name", name,
		"address", localIP,
	)

	body, err := json.Marshal(joinRequest{
		Token:   token,
		Name:    name,
		Address: localIP,
	})
	if err != nil {
		return fmt.Errorf("marshal join request: %w", err)
	}

	resp, err := client.Post(
		serverURL+"/api/v1/hosts/join",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("post join: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(respBody))
	}

	var jr joinResponse
	if err := json.NewDecoder(resp.Body).Decode(&jr); err != nil {
		return fmt.Errorf("decode join response: %w", err)
	}

	if jr.HostID == "" {
		return fmt.Errorf("server returned empty host_id")
	}

	cfg := &Config{
		ServerURL: serverURL,
		HostID:    jr.HostID,
		HostName:  name,
	}

	if err := saveConfig(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	logger.Info("join complete", "host_id", jr.HostID)
	return nil
}

// detectLocalIP returns the first non-loopback IPv4 address from the
// host's network interfaces.
func detectLocalIP() (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", fmt.Errorf("list interfaces: %w", err)
	}

	for _, iface := range ifaces {
		// Skip down, loopback, and point-to-point interfaces.
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			if ip == nil || ip.IsLoopback() {
				continue
			}

			// Return the first IPv4 address found.
			if ip4 := ip.To4(); ip4 != nil {
				return ip4.String(), nil
			}
		}
	}

	return "", fmt.Errorf("no suitable IPv4 address found")
}
