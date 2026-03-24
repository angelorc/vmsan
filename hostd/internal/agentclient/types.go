package agentclient

// RunParams are the parameters for the exec endpoint.
type RunParams struct {
	Cmd       string            `json:"cmd"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	Cwd       string            `json:"cwd,omitempty"`
	User      string            `json:"user,omitempty"`
	TimeoutMs int64             `json:"timeoutMs,omitempty"`
}

// RunEvent is a single event from the NDJSON exec stream.
type RunEvent struct {
	Type     string `json:"type"`               // "started", "stdout", "stderr", "exit", "timeout", "error"
	Data     string `json:"data,omitempty"`      // stdout/stderr payload
	ID       string `json:"id,omitempty"`        // command ID (on "started")
	PID      *int   `json:"pid,omitempty"`       // process PID (on "started")
	ExitCode *int   `json:"exitCode,omitempty"`  // exit code (on "exit")
	Ts       string `json:"ts,omitempty"`        // timestamp
	Error    string `json:"error,omitempty"`     // error message (on "error")
	Signal   string `json:"signal,omitempty"`    // signal name (on kill)
}

// WriteFileEntry is a single file to upload via tar.
type WriteFileEntry struct {
	Path    string
	Content []byte
	Mode    int64
}
