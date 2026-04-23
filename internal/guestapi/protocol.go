package guestapi

const (
	MessageTypeExec   = "exec"
	MessageTypeProxy  = "proxy"
	MessageTypeResult = "result"
	MessageTypeReady  = "ready"
	DefaultVSockPort  = 10789
)

type ExecRequest struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
	Command   string `json:"command"`
	Timeout   int    `json:"timeout"`
	Network   string `json:"network,omitempty"`
	Address   string `json:"address,omitempty"`
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
