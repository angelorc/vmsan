package health

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name:    "valid http",
			cfg:     Config{Type: CheckHTTP, Port: 8080, Path: "/health"},
			wantErr: false,
		},
		{
			name:    "http missing port",
			cfg:     Config{Type: CheckHTTP, Path: "/health"},
			wantErr: true,
		},
		{
			name:    "http missing path",
			cfg:     Config{Type: CheckHTTP, Port: 8080},
			wantErr: true,
		},
		{
			name:    "valid tcp",
			cfg:     Config{Type: CheckTCP, Port: 5432},
			wantErr: false,
		},
		{
			name:    "tcp missing port",
			cfg:     Config{Type: CheckTCP},
			wantErr: true,
		},
		{
			name:    "valid exec",
			cfg:     Config{Type: CheckExec, Command: "true"},
			wantErr: false,
		},
		{
			name:    "exec missing command",
			cfg:     Config{Type: CheckExec},
			wantErr: true,
		},
		{
			name:    "unknown type",
			cfg:     Config{Type: "grpc"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestApplyDefaults(t *testing.T) {
	cfg := Config{Type: CheckTCP, Port: 5432}
	applyDefaults(&cfg)

	if cfg.Interval != defaultInterval {
		t.Errorf("expected interval %d, got %d", defaultInterval, cfg.Interval)
	}
	if cfg.Timeout != defaultTimeout {
		t.Errorf("expected timeout %d, got %d", defaultTimeout, cfg.Timeout)
	}
	if cfg.Retries != defaultRetries {
		t.Errorf("expected retries %d, got %d", defaultRetries, cfg.Retries)
	}
	// StartPeriod 0 means "disabled", not "use default".
	if cfg.StartPeriod != 0 {
		t.Errorf("expected start_period 0 (disabled), got %d", cfg.StartPeriod)
	}
}

func TestApplyDefaultsNegativeStartPeriod(t *testing.T) {
	cfg := Config{Type: CheckTCP, Port: 5432, StartPeriod: -1}
	applyDefaults(&cfg)

	if cfg.StartPeriod != defaultStartPeriod {
		t.Errorf("expected start_period %d for negative input, got %d", defaultStartPeriod, cfg.StartPeriod)
	}
}

func TestApplyDefaultsPreservesValues(t *testing.T) {
	cfg := Config{
		Type:        CheckTCP,
		Port:        5432,
		Interval:    30,
		Timeout:     15,
		Retries:     5,
		StartPeriod: 60,
	}
	applyDefaults(&cfg)

	if cfg.Interval != 30 {
		t.Errorf("expected interval 30, got %d", cfg.Interval)
	}
	if cfg.Timeout != 15 {
		t.Errorf("expected timeout 15, got %d", cfg.Timeout)
	}
	if cfg.Retries != 5 {
		t.Errorf("expected retries 5, got %d", cfg.Retries)
	}
	if cfg.StartPeriod != 60 {
		t.Errorf("expected start_period 60, got %d", cfg.StartPeriod)
	}
}

func TestCheckerNotConfigured(t *testing.T) {
	c := NewChecker(testLogger())

	if c.Configured() {
		t.Error("expected not configured")
	}

	result := c.GetResult("1.0.0")
	if result.Status != StatusHealthy {
		t.Errorf("expected healthy, got %s", result.Status)
	}
	if result.Version != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %s", result.Version)
	}
	if len(result.Checks) != 0 {
		t.Errorf("expected no checks, got %d", len(result.Checks))
	}
}

func TestCheckerConfigureInvalidType(t *testing.T) {
	c := NewChecker(testLogger())
	err := c.Configure(Config{Type: "bad"})
	if err == nil {
		t.Error("expected error for invalid type")
	}
}

func TestCheckerExecHealthy(t *testing.T) {
	c := NewChecker(testLogger())
	err := c.Configure(Config{
		Type:        CheckExec,
		Command:     "true",
		Interval:    1,
		Timeout:     2,
		Retries:     1,
		StartPeriod: 0,
	})
	if err != nil {
		t.Fatalf("configure: %v", err)
	}
	defer c.Stop()

	if !c.Configured() {
		t.Error("expected configured")
	}

	// Wait for at least one check cycle.
	time.Sleep(500 * time.Millisecond)

	result := c.GetResult("dev")
	if result.Status != StatusHealthy {
		t.Errorf("expected healthy, got %s", result.Status)
	}
	if len(result.Checks) == 0 {
		t.Fatal("expected at least one check result")
	}
	if result.Checks[0].Type != CheckExec {
		t.Errorf("expected exec check type, got %s", result.Checks[0].Type)
	}
}

func TestCheckerExecUnhealthy(t *testing.T) {
	c := NewChecker(testLogger())
	err := c.Configure(Config{
		Type:        CheckExec,
		Command:     "false",
		Interval:    1,
		Timeout:     2,
		Retries:     1,
		StartPeriod: 0,
	})
	if err != nil {
		t.Fatalf("configure: %v", err)
	}
	defer c.Stop()

	// Wait for check to run and fail past retries.
	time.Sleep(500 * time.Millisecond)

	result := c.GetResult("dev")
	if result.Status != StatusUnhealthy {
		t.Errorf("expected unhealthy, got %s", result.Status)
	}
}

func TestCheckerHTTPHealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Extract port from the test server.
	_, portStr, _ := net.SplitHostPort(srv.Listener.Addr().String())
	var port int
	fmt.Sscanf(portStr, "%d", &port)

	c := NewChecker(testLogger())
	err := c.Configure(Config{
		Type:        CheckHTTP,
		Port:        port,
		Path:        "/",
		Interval:    1,
		Timeout:     2,
		Retries:     1,
		StartPeriod: 0,
	})
	if err != nil {
		t.Fatalf("configure: %v", err)
	}
	defer c.Stop()

	time.Sleep(500 * time.Millisecond)

	result := c.GetResult("dev")
	if result.Status != StatusHealthy {
		t.Errorf("expected healthy, got %s", result.Status)
	}
}

func TestCheckerTCPHealthy(t *testing.T) {
	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	// Accept connections in background.
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	var port int
	fmt.Sscanf(portStr, "%d", &port)

	c := NewChecker(testLogger())
	err = c.Configure(Config{
		Type:        CheckTCP,
		Port:        port,
		Interval:    1,
		Timeout:     2,
		Retries:     1,
		StartPeriod: 0,
	})
	if err != nil {
		t.Fatalf("configure: %v", err)
	}
	defer c.Stop()

	time.Sleep(500 * time.Millisecond)

	result := c.GetResult("dev")
	if result.Status != StatusHealthy {
		t.Errorf("expected healthy, got %s", result.Status)
	}
}

func TestCheckerTCPUnhealthy(t *testing.T) {
	// Use a port that is not listening.
	c := NewChecker(testLogger())
	err := c.Configure(Config{
		Type:        CheckTCP,
		Port:        19999,
		Interval:    1,
		Timeout:     1,
		Retries:     1,
		StartPeriod: 0,
	})
	if err != nil {
		t.Fatalf("configure: %v", err)
	}
	defer c.Stop()

	time.Sleep(1500 * time.Millisecond)

	result := c.GetResult("dev")
	if result.Status != StatusUnhealthy {
		t.Errorf("expected unhealthy, got %s", result.Status)
	}
}

func TestCheckerStartPeriod(t *testing.T) {
	c := NewChecker(testLogger())
	err := c.Configure(Config{
		Type:        CheckExec,
		Command:     "false",
		Interval:    1,
		Timeout:     2,
		Retries:     1,
		StartPeriod: 10, // Long start period.
	})
	if err != nil {
		t.Fatalf("configure: %v", err)
	}
	defer c.Stop()

	// Wait for check to run. During start period, should remain starting.
	time.Sleep(500 * time.Millisecond)

	result := c.GetResult("dev")
	if result.Status != StatusStarting {
		t.Errorf("expected starting during start period, got %s", result.Status)
	}
}

func TestCheckerReconfigure(t *testing.T) {
	c := NewChecker(testLogger())

	// First: failing check.
	err := c.Configure(Config{
		Type:        CheckExec,
		Command:     "false",
		Interval:    1,
		Timeout:     2,
		Retries:     1,
		StartPeriod: 0,
	})
	if err != nil {
		t.Fatalf("first configure: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	result := c.GetResult("dev")
	if result.Status != StatusUnhealthy {
		t.Errorf("expected unhealthy, got %s", result.Status)
	}

	// Reconfigure with a passing check.
	err = c.Configure(Config{
		Type:        CheckExec,
		Command:     "true",
		Interval:    1,
		Timeout:     2,
		Retries:     1,
		StartPeriod: 0,
	})
	if err != nil {
		t.Fatalf("second configure: %v", err)
	}
	defer c.Stop()

	time.Sleep(500 * time.Millisecond)

	result = c.GetResult("dev")
	if result.Status != StatusHealthy {
		t.Errorf("expected healthy after reconfigure, got %s", result.Status)
	}
}

func TestResultJSON(t *testing.T) {
	r := Result{
		Status:  StatusHealthy,
		Version: "1.0.0",
		Checks: []CheckResult{
			{
				Type:      CheckHTTP,
				Status:    StatusHealthy,
				Message:   "HTTP 200",
				LastCheck: "2026-03-21T00:00:00Z",
			},
		},
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded Result
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Status != StatusHealthy {
		t.Errorf("expected healthy, got %s", decoded.Status)
	}
	if decoded.Version != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %s", decoded.Version)
	}
	if len(decoded.Checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(decoded.Checks))
	}
	if decoded.Checks[0].Message != "HTTP 200" {
		t.Errorf("expected message 'HTTP 200', got %q", decoded.Checks[0].Message)
	}
}
