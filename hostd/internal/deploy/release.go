package deploy

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/angelorc/vmsan/hostd/internal/agentclient"
)

// ReleaseResult holds the outcome of running a release command (e.g. migrations).
type ReleaseResult struct {
	Success    bool   `json:"success"`
	ExitCode   int    `json:"exitCode"`
	Output     string `json:"output"`
	TimedOut   bool   `json:"timedOut"`
	DurationMs int64  `json:"durationMs"`
}

// ExecuteRelease runs a release command inside the VM via the agent.
// Default timeout is 5 minutes.
func ExecuteRelease(ctx context.Context, agent *agentclient.Client, releaseCmd string, env map[string]string) (*ReleaseResult, error) {
	start := time.Now()

	releaseCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	events, err := agent.Exec(releaseCtx, agentclient.RunParams{
		Cmd:       "sh",
		Args:      []string{"-c", releaseCmd},
		Env:       env,
		Cwd:       "/app",
		TimeoutMs: int64((5 * time.Minute).Milliseconds()),
	})
	if err != nil {
		return nil, fmt.Errorf("exec release: %w", err)
	}

	var output strings.Builder
	exitCode := -1
	timedOut := false

	for event := range events {
		switch event.Type {
		case "stdout", "stderr":
			output.WriteString(event.Data)
		case "exit":
			if event.ExitCode != nil {
				exitCode = *event.ExitCode
			}
		case "timeout":
			timedOut = true
		case "error":
			return &ReleaseResult{
				Success:    false,
				ExitCode:   -1,
				Output:     event.Error,
				DurationMs: time.Since(start).Milliseconds(),
			}, nil
		}
	}

	return &ReleaseResult{
		Success:    exitCode == 0 && !timedOut,
		ExitCode:   exitCode,
		Output:     output.String(),
		TimedOut:   timedOut,
		DurationMs: time.Since(start).Milliseconds(),
	}, nil
}
