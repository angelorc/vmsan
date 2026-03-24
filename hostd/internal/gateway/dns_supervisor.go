package gateway

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"time"
)

// DNSSupervisor manages per-VM dnsproxy sidecar processes with automatic
// restart on crash.
type DNSSupervisor struct {
	mu        sync.RWMutex
	processes map[string]*supervisedProcess // vmId -> process
	logger    *slog.Logger
}

type supervisedProcess struct {
	cmd       *exec.Cmd
	vmId      string
	port      int
	args      []string // dnsproxy args for restart
	restarts  int
	lastStart time.Time
	cancel    context.CancelFunc
	done      chan struct{}
}

// Restart policy constants
const (
	maxRestarts       = 10
	restartWindow     = 5 * time.Minute
	maxBackoff        = 30 * time.Second
	stableRunDuration = 60 * time.Second
)

// NewDNSSupervisor creates a DNS supervisor.
func NewDNSSupervisor(logger *slog.Logger) *DNSSupervisor {
	return &DNSSupervisor{
		processes: make(map[string]*supervisedProcess),
		logger:    logger,
	}
}

// Start launches a dnsproxy process for a VM and supervises it.
func (s *DNSSupervisor) Start(vmId string, port int, args []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Stop existing if any
	if existing, ok := s.processes[vmId]; ok {
		existing.cancel()
		<-existing.done
	}

	ctx, cancel := context.WithCancel(context.Background())
	proc := &supervisedProcess{
		vmId:   vmId,
		port:   port,
		args:   args,
		cancel: cancel,
		done:   make(chan struct{}),
	}
	s.processes[vmId] = proc

	go s.supervise(ctx, proc)

	return nil
}

// Stop stops the dnsproxy process for a VM.
func (s *DNSSupervisor) Stop(vmId string) error {
	s.mu.Lock()
	proc, ok := s.processes[vmId]
	if !ok {
		s.mu.Unlock()
		return nil
	}
	delete(s.processes, vmId)
	s.mu.Unlock()

	proc.cancel()
	<-proc.done
	return nil
}

// StopAll stops all supervised processes.
func (s *DNSSupervisor) StopAll() {
	s.mu.Lock()
	procs := make([]*supervisedProcess, 0, len(s.processes))
	for _, proc := range s.processes {
		procs = append(procs, proc)
	}
	s.processes = make(map[string]*supervisedProcess)
	s.mu.Unlock()

	for _, proc := range procs {
		proc.cancel()
		<-proc.done
	}
}

// Count returns the number of supervised processes.
func (s *DNSSupervisor) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.processes)
}

// supervise runs the process and restarts it on crash.
func (s *DNSSupervisor) supervise(ctx context.Context, proc *supervisedProcess) {
	defer close(proc.done)

	var recentRestarts int
	var windowStart time.Time

	for {
		select {
		case <-ctx.Done():
			// Graceful shutdown — kill current process
			if proc.cmd != nil && proc.cmd.Process != nil {
				proc.cmd.Process.Kill()
			}
			return
		default:
		}

		// Reset restart counter after stable run
		if time.Since(windowStart) > restartWindow {
			recentRestarts = 0
			windowStart = time.Now()
		}

		// Check if we've exceeded restart limit
		if recentRestarts >= maxRestarts {
			s.logger.Error("dnsproxy exceeded max restarts, giving up",
				"vmId", proc.vmId,
				"restarts", recentRestarts,
				"window", restartWindow,
			)
			return
		}

		// Start the process
		proc.cmd = exec.CommandContext(ctx, proc.args[0], proc.args[1:]...)
		proc.cmd.Stderr = os.Stderr
		proc.cmd.Stdout = os.Stdout
		proc.lastStart = time.Now()

		s.logger.Info("starting dnsproxy", "vmId", proc.vmId, "port", proc.port, "restart", proc.restarts, "args", proc.args)

		err := proc.cmd.Start()
		if err != nil {
			s.logger.Error("dnsproxy start failed", "vmId", proc.vmId, "error", err)
			recentRestarts++
			proc.restarts++
			backoff := calculateBackoff(recentRestarts)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			continue
		}

		// Wait for process to exit
		waitDone := make(chan error, 1)
		go func() {
			waitDone <- proc.cmd.Wait()
		}()

		select {
		case <-ctx.Done():
			if proc.cmd.Process != nil {
				proc.cmd.Process.Kill()
			}
			<-waitDone
			return
		case err := <-waitDone:
			if ctx.Err() != nil {
				return // context cancelled
			}
			if err == nil {
				s.logger.Info("dnsproxy exited cleanly", "vmId", proc.vmId)
				return // clean exit, don't restart
			}

			// Crash — restart with backoff
			runDuration := time.Since(proc.lastStart)
			if runDuration > stableRunDuration {
				recentRestarts = 0 // was stable, reset counter
			}

			recentRestarts++
			proc.restarts++

			s.logger.Warn("dnsproxy crashed, restarting",
				"vmId", proc.vmId,
				"error", err,
				"restarts", proc.restarts,
				"runDuration", runDuration,
			)

			backoff := calculateBackoff(recentRestarts)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
		}
	}
}

// calculateBackoff returns exponential backoff: 1s, 2s, 4s, 8s, 16s, 30s (capped).
func calculateBackoff(attempt int) time.Duration {
	backoff := time.Duration(1<<uint(attempt-1)) * time.Second
	if backoff > maxBackoff {
		return maxBackoff
	}
	return backoff
}
