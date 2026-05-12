package logging

import (
	"os"
	"strings"
)

type Level int

const (
	LevelQuiet Level = iota
	LevelInfo
	LevelDebug
)

const defaultLevel = LevelDebug

func CurrentLevel() Level {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("AIR_LOG_LEVEL"))) {
	case "quiet", "error", "off":
		return LevelQuiet
	case "info":
		return LevelInfo
	case "debug", "trace", "":
		return defaultLevel
	default:
		return defaultLevel
	}
}

func DebugEnabled() bool {
	return CurrentLevel() >= LevelDebug
}

func InfoEnabled() bool {
	return CurrentLevel() >= LevelInfo
}
