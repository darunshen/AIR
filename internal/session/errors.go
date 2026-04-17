package session

type RunErrorType string

const (
	RunErrorTypeNone            RunErrorType = ""
	RunErrorTypeInvalidArgument RunErrorType = "invalid_argument"
	RunErrorTypeStartup         RunErrorType = "startup_error"
	RunErrorTypeTransport       RunErrorType = "transport_error"
	RunErrorTypeExec            RunErrorType = "exec_error"
	RunErrorTypeTimeout         RunErrorType = "timeout"
	RunErrorTypeCleanup         RunErrorType = "cleanup_error"
)
