package output

import (
	"strings"
	"testing"
	"time"
)

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		name  string
		input int64
		want  string
	}{
		{"zero", 0, "0 B"},
		{"small bytes", 500, "500 B"},
		{"one KB", 1024, "1.0 KB"},
		{"one MB", 1048576, "1.0 MB"},
		{"one GB", 1073741824, "1.0 GB"},
		{"1.5 KB", 1536, "1.5 KB"},
		{"one TB", 1099511627776, "1.0 TB"},
		{"just under 1 KB", 1023, "1023 B"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatBytes(tt.input)
			if got != tt.want {
				t.Errorf("FormatBytes(%d) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestTimeAgo(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		contains string
	}{
		{"30 seconds", 30 * time.Second, "30s ago"},
		{"5 minutes", 5 * time.Minute, "5m ago"},
		{"2 hours", 2 * time.Hour, "2h ago"},
		{"48 hours", 48 * time.Hour, "2d ago"},
		{"1 second", 1 * time.Second, "1s ago"},
		{"59 seconds", 59 * time.Second, "59s ago"},
		{"1 minute", 1 * time.Minute, "1m ago"},
		{"23 hours", 23 * time.Hour, "23h ago"},
		{"72 hours", 72 * time.Hour, "3d ago"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			past := time.Now().Add(-tt.duration)
			got := TimeAgo(past)
			if !strings.Contains(got, tt.contains) {
				t.Errorf("TimeAgo(now - %v) = %q, want string containing %q", tt.duration, got, tt.contains)
			}
			if !strings.HasSuffix(got, "ago") {
				t.Errorf("TimeAgo() = %q, should end with 'ago'", got)
			}
		})
	}
}

func TestTimeRemaining(t *testing.T) {
	// Add a small buffer to avoid truncation issues (time passes between
	// time.Now().Add(d) and the call to TimeRemaining).
	tests := []struct {
		name     string
		duration time.Duration
		contains string
	}{
		{"30 seconds", 31 * time.Second, "30s"},
		{"5 minutes", 5*time.Minute + time.Second, "5m"},
		{"2 hours", 2*time.Hour + time.Second, "2h"},
		{"1 second", 2 * time.Second, "s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			future := time.Now().Add(tt.duration)
			got := TimeRemaining(future)
			if !strings.Contains(got, tt.contains) {
				t.Errorf("TimeRemaining(now + %v) = %q, want string containing %q", tt.duration, got, tt.contains)
			}
		})
	}
}

func TestTimeRemaining_Expired(t *testing.T) {
	past := time.Now().Add(-5 * time.Second)
	got := TimeRemaining(past)
	if got != "expired" {
		t.Errorf("TimeRemaining(past) = %q, want %q", got, "expired")
	}
}

func TestTimeRemaining_JustExpired(t *testing.T) {
	// Exactly now or slightly past should be "expired".
	past := time.Now().Add(-1 * time.Millisecond)
	got := TimeRemaining(past)
	if got != "expired" {
		t.Errorf("TimeRemaining(just past) = %q, want %q", got, "expired")
	}
}
