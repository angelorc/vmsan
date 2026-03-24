package agentclient

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Client talks to the in-VM agent HTTP API.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// New creates an agent client for the given base URL and auth token.
func New(baseURL, token string) *Client {
	return &Client{
		baseURL:    baseURL,
		token:      token,
		httpClient: &http.Client{},
	}
}

// Health performs a health check against the agent.
func (c *Client) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("agent health check failed (%d): %s", resp.StatusCode, string(body))
	}
	return nil
}

// Exec sends a command to the agent and returns a channel of NDJSON events.
// The channel is closed when the stream ends. The caller should read all events.
func (c *Client) Exec(ctx context.Context, params RunParams) (<-chan RunEvent, error) {
	body, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/exec", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		text, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("agent exec failed (%d): %s", resp.StatusCode, string(text))
	}

	events := make(chan RunEvent)
	go func() {
		defer close(events)
		defer resp.Body.Close()
		scanner := bufio.NewScanner(resp.Body)
		// Allow large lines (agent may send big stdout chunks).
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := bytes.TrimSpace(scanner.Bytes())
			if len(line) == 0 {
				continue
			}
			var event RunEvent
			if err := json.Unmarshal(line, &event); err != nil {
				continue
			}
			select {
			case events <- event:
			case <-ctx.Done():
				return
			}
		}
	}()

	return events, nil
}

// KillCommand sends a kill signal to a running command.
func (c *Client) KillCommand(ctx context.Context, cmdID, signal string) error {
	var body io.Reader
	if signal != "" {
		data, _ := json.Marshal(map[string]string{"signal": signal})
		body = bytes.NewReader(data)
	}

	url := fmt.Sprintf("%s/exec/%s/kill", c.baseURL, cmdID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		text, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("agent kill failed (%d): %s", resp.StatusCode, string(text))
	}
	return nil
}

// WriteFiles uploads files to the agent as a tar+gzip archive.
// extractDir sets the X-Extract-Dir header (destination directory in the VM).
func (c *Client) WriteFiles(ctx context.Context, files []WriteFileEntry, extractDir string) error {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	for _, f := range files {
		mode := f.Mode
		if mode == 0 {
			mode = 0644
		}
		hdr := &tar.Header{
			Name: f.Path,
			Size: int64(len(f.Content)),
			Mode: mode,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return fmt.Errorf("tar write header: %w", err)
		}
		if _, err := tw.Write(f.Content); err != nil {
			return fmt.Errorf("tar write content: %w", err)
		}
	}
	if err := tw.Close(); err != nil {
		return fmt.Errorf("tar close: %w", err)
	}
	if err := gw.Close(); err != nil {
		return fmt.Errorf("gzip close: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/files/write", &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/gzip")
	req.Header.Set("Authorization", "Bearer "+c.token)
	if extractDir != "" {
		req.Header.Set("X-Extract-Dir", extractDir)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		text, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("agent writeFiles failed (%d): %s", resp.StatusCode, string(text))
	}
	return nil
}

// ReadFile downloads a single file from the VM. Returns nil, nil if the file
// is not found (HTTP 404).
func (c *Client) ReadFile(ctx context.Context, remotePath string) ([]byte, error) {
	body, err := json.Marshal(map[string]string{"path": remotePath})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/files/read", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		text, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("agent readFile failed (%d): %s", resp.StatusCode, string(text))
	}

	return io.ReadAll(resp.Body)
}
