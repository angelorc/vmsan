package deploy

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/angelorc/vmsan/hostd/internal/agentclient"
)

// BuildResult holds the outcome of running a build command in a VM.
type BuildResult struct {
	Success    bool   `json:"success"`
	ExitCode   int    `json:"exitCode"`
	Output     string `json:"output"`
	DurationMs int64  `json:"durationMs"`
}

// ExecuteBuild runs a build command inside the VM via the agent.
// Default timeout is 10 minutes.
func ExecuteBuild(ctx context.Context, agent *agentclient.Client, buildCmd string, env map[string]string) (*BuildResult, error) {
	start := time.Now()

	buildCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	events, err := agent.Exec(buildCtx, agentclient.RunParams{
		Cmd:       "sh",
		Args:      []string{"-c", buildCmd},
		Env:       env,
		Cwd:       "/app",
		TimeoutMs: int64((10 * time.Minute).Milliseconds()),
	})
	if err != nil {
		return nil, fmt.Errorf("exec build: %w", err)
	}

	var output strings.Builder
	exitCode := -1

	for event := range events {
		switch event.Type {
		case "stdout", "stderr":
			output.WriteString(event.Data)
		case "exit":
			if event.ExitCode != nil {
				exitCode = *event.ExitCode
			}
		case "error":
			return &BuildResult{
				Success:    false,
				ExitCode:   -1,
				Output:     event.Error,
				DurationMs: time.Since(start).Milliseconds(),
			}, nil
		case "timeout":
			return &BuildResult{
				Success:    false,
				ExitCode:   -1,
				Output:     "build timed out",
				DurationMs: time.Since(start).Milliseconds(),
			}, nil
		}
	}

	return &BuildResult{
		Success:    exitCode == 0,
		ExitCode:   exitCode,
		Output:     output.String(),
		DurationMs: time.Since(start).Milliseconds(),
	}, nil
}

// StartApp starts the application as a detached process inside the VM.
func StartApp(ctx context.Context, agent *agentclient.Client, startCmd string, env map[string]string) error {
	events, err := agent.Exec(ctx, agentclient.RunParams{
		Cmd:  "sh",
		Args: []string{"-c", fmt.Sprintf("nohup %s > /var/log/app.log 2>&1 &", startCmd)},
		Env:  env,
		Cwd:  "/app",
	})
	if err != nil {
		return fmt.Errorf("start app: %w", err)
	}

	// Drain events
	for event := range events {
		if event.Type == "error" {
			return fmt.Errorf("start failed: %s", event.Error)
		}
	}

	return nil
}
