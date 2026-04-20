package llm

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

type Planner interface {
	NextAction(context.Context, PlanRequest) (*PlanAction, error)
}

type PlanRequest struct {
	TaskName           string            `json:"task_name"`
	Goal               string            `json:"goal"`
	Mode               string            `json:"mode"`
	AllowedActionTypes []string          `json:"allowed_action_types"`
	Provider           string            `json:"provider"`
	SessionID          string            `json:"session_id,omitempty"`
	Step               int               `json:"step"`
	MaxSteps           int               `json:"max_steps"`
	History            []StepObservation `json:"history"`
}

type StepObservation struct {
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	Command      string `json:"command,omitempty"`
	RequestID    string `json:"request_id,omitempty"`
	Stdout       string `json:"stdout,omitempty"`
	Stderr       string `json:"stderr,omitempty"`
	ExitCode     int    `json:"exit_code,omitempty"`
	DurationMS   int64  `json:"duration_ms,omitempty"`
	Timeout      bool   `json:"timeout,omitempty"`
	Success      bool   `json:"success"`
	ErrorMessage string `json:"error_message,omitempty"`
	Note         string `json:"note,omitempty"`
}

type PlanAction struct {
	Type          string `json:"type"`
	Command       string `json:"command"`
	Reason        string `json:"reason"`
	FinishSuccess bool   `json:"finish_success"`
	FinishSummary string `json:"finish_summary"`
}

type Config struct {
	Provider  string
	Model     string
	Reasoning string
	APIKey    string
	BaseURL   string
	Timeout   time.Duration
	Logger    func(format string, args ...any)
}

func ResolveConfigFromEnv() Config {
	return Config{
		Provider:  getenvDefault("AIR_AGENT_PROVIDER", "openai"),
		Model:     os.Getenv("AIR_AGENT_MODEL"),
		Reasoning: getenvDefault("AIR_AGENT_REASONING", "medium"),
		Timeout:   60 * time.Second,
	}
}

func New(cfg Config) (Planner, error) {
	cfg = NormalizeConfig(cfg)
	if cfg.Provider == "" {
		cfg.Provider = "openai"
	}
	switch strings.ToLower(cfg.Provider) {
	case "openai":
		return NewOpenAIPlanner(cfg)
	case "deepseek":
		return NewDeepSeekPlanner(cfg)
	default:
		return nil, fmt.Errorf("unsupported llm provider: %s", cfg.Provider)
	}
}

func NormalizeConfig(cfg Config) Config {
	if cfg.Provider == "" {
		cfg.Provider = "openai"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60 * time.Second
	}

	switch strings.ToLower(cfg.Provider) {
	case "deepseek":
		if cfg.Model == "" {
			cfg.Model = getenvDefault("AIR_AGENT_MODEL", "deepseek-chat")
		}
		if cfg.APIKey == "" {
			cfg.APIKey = os.Getenv("DEEPSEEK_API_KEY")
		}
		if cfg.BaseURL == "" {
			cfg.BaseURL = getenvDefault("DEEPSEEK_BASE_URL", "https://api.deepseek.com")
		}
	case "openai":
		fallthrough
	default:
		if cfg.Model == "" {
			cfg.Model = getenvDefault("AIR_AGENT_MODEL", "gpt-5.4-mini")
		}
		if cfg.APIKey == "" {
			cfg.APIKey = os.Getenv("OPENAI_API_KEY")
		}
		if cfg.BaseURL == "" {
			cfg.BaseURL = getenvDefault("OPENAI_BASE_URL", "https://api.openai.com/v1")
		}
	}
	return cfg
}

func getenvDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func tracef(cfg Config, format string, args ...any) {
	if cfg.Logger == nil {
		return
	}
	cfg.Logger(format, args...)
}

func previewText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + "...(truncated)"
}
