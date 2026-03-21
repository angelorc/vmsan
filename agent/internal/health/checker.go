package health

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	defaultInterval    = 10
	defaultTimeout     = 5
	defaultRetries     = 3
	defaultStartPeriod = 30
)

// Checker runs periodic health checks and tracks status.
type Checker struct {
	mu sync.Mutex

	config     Config
	configured bool

	status       Status
	lastResult   CheckResult
	failCount    int
	startedAt    time.Time
	cancel       context.CancelFunc
	logger       *slog.Logger
	httpClient   *http.Client
}

// NewChecker creates a Checker that is not yet configured.
// Call Configure to start running checks.
func NewChecker(logger *slog.Logger) *Checker {
	return &Checker{
		logger: logger,
	}
}

// Configured reports whether a health check has been configured.
func (c *Checker) Configured() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.configured
}

// Configure applies a health check config and starts the check loop.
// If a previous check loop is running, it is stopped first.
func (c *Checker) Configure(cfg Config) error {
	if err := validateConfig(cfg); err != nil {
		return err
	}
	applyDefaults(&cfg)

	c.mu.Lock()
	defer c.mu.Unlock()

	// Stop any existing loop.
	if c.cancel != nil {
		c.cancel()
	}

	c.config = cfg
	c.configured = true
	c.status = StatusStarting
	c.failCount = 0
	c.startedAt = time.Now()
	c.lastResult = CheckResult{}
	c.httpClient = &http.Client{
		Timeout: time.Duration(cfg.Timeout) * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel

	go c.loop(ctx)

	c.logger.Info("health check configured",
		"type", string(cfg.Type),
		"interval", cfg.Interval,
		"timeout", cfg.Timeout,
		"retries", cfg.Retries,
		"start_period", cfg.StartPeriod,
	)

	return nil
}

// GetResult returns the current health status.
func (c *Checker) GetResult(version string) Result {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.configured {
		return Result{
			Status:  StatusHealthy,
			Version: version,
		}
	}

	res := Result{
		Status:  c.status,
		Version: version,
	}

	if c.lastResult.Type != "" {
		res.Checks = []CheckResult{c.lastResult}
	}

	return res
}

// Stop cancels the check loop.
func (c *Checker) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
}

func (c *Checker) loop(ctx context.Context) {
	c.mu.Lock()
	interval := time.Duration(c.config.Interval) * time.Second
	c.mu.Unlock()

	// Run the first check immediately.
	c.runCheck(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.runCheck(ctx)
		}
	}
}

func (c *Checker) runCheck(ctx context.Context) {
	c.mu.Lock()
	cfg := c.config
	c.mu.Unlock()

	var msg string
	var ok bool

	switch cfg.Type {
	case CheckHTTP:
		ok, msg = c.checkHTTP(ctx, cfg)
	case CheckTCP:
		ok, msg = c.checkTCP(ctx, cfg)
	case CheckExec:
		ok, msg = c.checkExec(ctx, cfg)
	default:
		msg = fmt.Sprintf("unknown check type: %s", cfg.Type)
	}

	now := time.Now()
	checkStatus := StatusHealthy
	if !ok {
		checkStatus = StatusUnhealthy
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.lastResult = CheckResult{
		Type:      cfg.Type,
		Status:    checkStatus,
		Message:   msg,
		LastCheck: now.UTC().Format(time.RFC3339),
	}

	if ok {
		c.failCount = 0
		c.status = StatusHealthy
		c.logger.Debug("health check passed", "type", string(cfg.Type), "message", msg)
	} else {
		c.failCount++
		inStartPeriod := cfg.StartPeriod > 0 && now.Before(c.startedAt.Add(time.Duration(cfg.StartPeriod)*time.Second))

		if inStartPeriod {
			// During start period, stay in starting state on failure.
			if c.status != StatusHealthy {
				c.status = StatusStarting
			}
			c.logger.Debug("health check failed (start period)",
				"type", string(cfg.Type),
				"fail_count", c.failCount,
				"message", msg,
			)
		} else if c.failCount >= cfg.Retries {
			c.status = StatusUnhealthy
			c.logger.Warn("health check unhealthy",
				"type", string(cfg.Type),
				"fail_count", c.failCount,
				"message", msg,
			)
		} else {
			c.logger.Debug("health check failed",
				"type", string(cfg.Type),
				"fail_count", c.failCount,
				"retries", cfg.Retries,
				"message", msg,
			)
		}
	}
}

func (c *Checker) checkHTTP(ctx context.Context, cfg Config) (bool, string) {
	url := fmt.Sprintf("http://localhost:%d%s", cfg.Port, cfg.Path)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, fmt.Sprintf("create request: %s", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Sprintf("request failed: %s", err)
	}
	resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return true, fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	return false, fmt.Sprintf("HTTP %d", resp.StatusCode)
}

func (c *Checker) checkTCP(_ context.Context, cfg Config) (bool, string) {
	addr := fmt.Sprintf("localhost:%d", cfg.Port)
	timeout := time.Duration(cfg.Timeout) * time.Second

	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return false, fmt.Sprintf("dial failed: %s", err)
	}
	conn.Close()
	return true, "connection established"
}

func (c *Checker) checkExec(ctx context.Context, cfg Config) (bool, string) {
	timeout := time.Duration(cfg.Timeout) * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Split command into parts. For simplicity, use shell -c.
	cmd := exec.CommandContext(ctx, "sh", "-c", cfg.Command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if msg == "" {
			msg = err.Error()
		}
		return false, msg
	}

	msg := strings.TrimSpace(string(output))
	if msg == "" {
		msg = "exit 0"
	}
	return true, msg
}

func validateConfig(cfg Config) error {
	switch cfg.Type {
	case CheckHTTP:
		if cfg.Port <= 0 {
			return fmt.Errorf("http check requires port > 0")
		}
		if cfg.Path == "" {
			return fmt.Errorf("http check requires path")
		}
	case CheckTCP:
		if cfg.Port <= 0 {
			return fmt.Errorf("tcp check requires port > 0")
		}
	case CheckExec:
		if cfg.Command == "" {
			return fmt.Errorf("exec check requires command")
		}
	default:
		return fmt.Errorf("unsupported check type: %q", cfg.Type)
	}
	return nil
}

func applyDefaults(cfg *Config) {
	if cfg.Interval <= 0 {
		cfg.Interval = defaultInterval
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	if cfg.Retries <= 0 {
		cfg.Retries = defaultRetries
	}
	// StartPeriod: 0 means "no grace period" (disabled).
	// Only apply default when negative (invalid input).
	if cfg.StartPeriod < 0 {
		cfg.StartPeriod = defaultStartPeriod
	}
}
