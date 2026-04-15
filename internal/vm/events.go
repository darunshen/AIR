package vm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type runtimeEvent struct {
	Timestamp string         `json:"timestamp"`
	Event     string         `json:"event"`
	Provider  string         `json:"provider"`
	SessionID string         `json:"session_id"`
	Fields    map[string]any `json:"fields,omitempty"`
}

func appendRuntimeEvent(path, provider, sessionID, event string, fields map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	entry := runtimeEvent{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Event:     event,
		Provider:  provider,
		SessionID: sessionID,
		Fields:    fields,
	}
	return json.NewEncoder(file).Encode(entry)
}
