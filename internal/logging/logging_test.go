package logging

import "testing"

func TestCurrentLevelDefaultsToDebug(t *testing.T) {
	t.Setenv("AIR_LOG_LEVEL", "")
	if got := CurrentLevel(); got != LevelDebug {
		t.Fatalf("expected default debug level, got %v", got)
	}
	if !DebugEnabled() {
		t.Fatal("expected debug enabled by default")
	}
}

func TestCurrentLevelSupportsQuiet(t *testing.T) {
	t.Setenv("AIR_LOG_LEVEL", "quiet")
	if got := CurrentLevel(); got != LevelQuiet {
		t.Fatalf("expected quiet level, got %v", got)
	}
	if DebugEnabled() {
		t.Fatal("expected debug disabled in quiet mode")
	}
	if InfoEnabled() {
		t.Fatal("expected info disabled in quiet mode")
	}
}
