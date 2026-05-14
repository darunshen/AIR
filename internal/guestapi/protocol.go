package guestapi

const (
	MessageTypeExec   = "exec"
	MessageTypeChunk  = "exec_chunk"
	MessageTypePTY    = "pty"
	MessageTypeResize = "pty_resize"
	MessageTypeProxy  = "proxy"
	MessageTypeResult = "result"
	MessageTypeReady  = "ready"
	DefaultVSockPort  = 10789
)

type ExecRequest struct {
	Type      string            `json:"type"`
	RequestID string            `json:"request_id"`
	Command   string            `json:"command"`
	Timeout   int               `json:"timeout"`
	Stream    bool              `json:"stream,omitempty"`
	Network   string            `json:"network,omitempty"`
	Address   string            `json:"address,omitempty"`
	Rows      uint16            `json:"rows,omitempty"`
	Cols      uint16            `json:"cols,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
}

type ExecChunk struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
	Stream    string `json:"stream"`
	Data      string `json:"data"`
}

type ReadyResult struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
}

type ExecResult struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
	ExitCode  int    `json:"exit_code"`
	TimedOut  bool   `json:"timed_out,omitempty"`
	Error     string `json:"error,omitempty"`
}

type ProxyResult struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
}

type PTYResult struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
}
