package logger

import (
	"bytes"
	"strings"
	"testing"
)

func TestLogger(t *testing.T)  {
	var buf bytes.Buffer

	logger := New("test-service")
	logger.info.SetOutput(&buf)

	logger.Info("test message")

	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Errorf("Expected log to contain 'test message', got: %s", output)
	}

	if !strings.Contains(output, "[test-service]"){
		t.Errorf("Expected log to contain service name, got: %s", output)
	}
}