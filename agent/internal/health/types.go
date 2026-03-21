package health

// CheckType represents the type of health check.
type CheckType string

const (
	CheckHTTP CheckType = "http"
	CheckTCP  CheckType = "tcp"
	CheckExec CheckType = "exec"
)

// Config defines a health check configuration.
type Config struct {
	Type        CheckType `json:"type"`
	Path        string    `json:"path,omitempty"`         // HTTP path
	Port        int       `json:"port,omitempty"`         // HTTP/TCP port
	Command     string    `json:"command,omitempty"`       // exec command
	Interval    int       `json:"interval,omitempty"`      // seconds, default 10
	Timeout     int       `json:"timeout,omitempty"`       // seconds, default 5
	Retries     int       `json:"retries,omitempty"`       // default 3
	StartPeriod int       `json:"start_period,omitempty"` // seconds, default 30
}

// Status represents current health state.
type Status string

const (
	StatusStarting  Status = "starting"
	StatusHealthy   Status = "healthy"
	StatusUnhealthy Status = "unhealthy"
)

// Result is the response from GET /health when checks are configured.
type Result struct {
	Status  Status        `json:"status"`
	Version string        `json:"version"`
	Checks  []CheckResult `json:"checks,omitempty"`
}

// CheckResult holds the outcome of a single health check.
type CheckResult struct {
	Type      CheckType `json:"type"`
	Status    Status    `json:"status"`
	Message   string    `json:"message,omitempty"`
	LastCheck string    `json:"last_check,omitempty"`
}
