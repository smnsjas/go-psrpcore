package runspace

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/google/uuid"
)

func TestSetSlogLogger(t *testing.T) {
	// Setup buffer to capture logs
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	logger := slog.New(handler)

	// Create pool and set logger
	pool := New(nil, uuid.Nil)
	err := pool.SetSlogLogger(logger)
	if err != nil {
		t.Fatalf("SetSlogLogger failed: %v", err)
	}

	// Trigger a log message (by changing state, if that logs, or just calling internal logf via reflection?
	// Actually, logf is private. We need to trigger something public that logs.
	// Open() logs "opening runspace pool".

	// Create a mock context to avoid network calls if possible, or expect error
	// But Open requires a client.

	// Alternative: Test SetSlogLogger state check directly
	if pool.slogLogger == nil {
		t.Fatal("slogLogger not set")
	}

	// We can't easily trigger a log without a full mock client setup which is complex.
	// But we can verify strict state behavior.
	pool.state = StateOpened
	err = pool.SetSlogLogger(logger)
	if err != ErrInvalidState {
		t.Errorf("expected ErrInvalidState, got %v", err)
	}
}

func TestSlogOutput(t *testing.T) {
	// This test relies on internal access (same package)
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	logger := slog.New(handler)

	pool := New(nil, uuid.Nil)
	pool.SetSlogLogger(logger)

	// Call private logf directly since we are in same package
	pool.logf("test message %d", 123)

	// Parse JSON output
	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("failed to parse log JSON: %v", err)
	}

	// Verify fields
	if logEntry["msg"] != "test message 123" {
		t.Errorf("expected msg 'test message 123', got '%v'", logEntry["msg"])
	}
	if logEntry["level"] != "DEBUG" {
		t.Errorf("expected level DEBUG, got %v", logEntry["level"])
	}
	if _, ok := logEntry["runspace_id"]; !ok {
		t.Error("missing runspace_id field")
	}
	if _, ok := logEntry["time"]; !ok {
		t.Error("missing time field")
	}
}
