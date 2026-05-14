package log

import (
	"bytes"
	"strings"
	"testing"
)

func TestRedactPassword(t *testing.T) {
	result := Redact("password=secret123")
	if strings.Contains(result, "secret123") {
		t.Errorf("password not redacted: %s", result)
	}
	if !strings.Contains(result, "***") {
		t.Errorf("expected *** placeholder: %s", result)
	}
}

func TestRedactPasswordWithQuotes(t *testing.T) {
	result := Redact(`password="s3cr3t"`)
	if strings.Contains(result, "s3cr3t") {
		t.Errorf("password not redacted: %s", result)
	}
}

func TestRedactTokenQueryParam(t *testing.T) {
	result := Redact("https://host.com?token=abc123&user=me")
	if strings.Contains(result, "abc123") {
		t.Errorf("token not redacted: %s", result)
	}
}

func TestRedactSQLServerDSN(t *testing.T) {
	result := Redact("sqlserver://sa:StrongPass@localhost:1433")
	if strings.Contains(result, "StrongPass") {
		t.Errorf("DSN password not redacted: %s", result)
	}
	if !strings.Contains(result, "sa:***@") {
		t.Errorf("expected sa:***@: %s", result)
	}
}

func TestRedactJSONField(t *testing.T) {
	result := Redact(`{"password":"s3cr3t","user":"me"}`)
	if strings.Contains(result, "s3cr3t") {
		t.Errorf("JSON password not redacted: %s", result)
	}
}

func TestRedactNoop(t *testing.T) {
	input := "plan complete"
	result := Redact(input)
	if result != input {
		t.Errorf("unexpected redaction: %q -> %q", input, result)
	}
}

func TestRedactAccessToken(t *testing.T) {
	result := Redact("access_token=ghp_1234567890abcdef")
	if strings.Contains(result, "ghp_") {
		t.Errorf("access token not redacted: %s", result)
	}
}

func TestRedactClientSecret(t *testing.T) {
	result := Redact("client_secret = 'my-secret'")
	if strings.Contains(result, "my-secret") {
		t.Errorf("client secret not redacted: %s", result)
	}
}

func TestLoggerPlainText(t *testing.T) {
	var buf bytes.Buffer
	l := New(false, "debug", &buf)
	l.Info("run.started", "plan begins")

	output := buf.String()
	if !strings.Contains(output, "INFO") {
		t.Errorf("missing INFO level: %s", output)
	}
	if !strings.Contains(output, "plan begins") {
		t.Errorf("missing message: %s", output)
	}
}

func TestLoggerJSON(t *testing.T) {
	var buf bytes.Buffer
	l := New(true, "debug", &buf)
	l.Info("run.started", `{"ok":true}`)

	output := buf.String()
	if !strings.Contains(output, `"level":"info"`) {
		t.Errorf("expected JSON level: %s", output)
	}
	if !strings.Contains(output, `"event":"run.started"`) {
		t.Errorf("expected JSON event: %s", output)
	}
}

func TestLoggerLevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	l := New(false, "warn", &buf)
	l.Debug("debug", "should not appear")
	l.Info("info", "should not appear")
	l.Warn("warn", "should appear")

	output := buf.String()
	if strings.Contains(output, "debug") {
		t.Errorf("debug should be filtered: %s", output)
	}
	if strings.Contains(output, "info") {
		t.Errorf("info should be filtered: %s", output)
	}
	if !strings.Contains(output, "warn") {
		t.Errorf("warn should appear: %s", output)
	}
}

func TestLoggerErrorEnvelope_PlainTextRedacts(t *testing.T) {
	var buf bytes.Buffer
	l := New(false, "debug", &buf)
	l.ErrorEnvelope("run.failed", "ERROR plan: password=secret123")

	output := buf.String()
	if strings.Contains(output, "secret123") {
		t.Errorf("envelope password not redacted: %s", output)
	}
}

func TestLoggerErrorEnvelope_JSON(t *testing.T) {
	var buf bytes.Buffer
	l := New(true, "debug", &buf)
	l.ErrorEnvelope("run.failed", "ERROR plan: sql_root=/sql")

	output := buf.String()
	if !strings.Contains(output, `"level":"error"`) {
		t.Errorf("expected JSON error level: %s", output)
	}
	if !strings.Contains(output, `"event":"run.failed"`) {
		t.Errorf("expected JSON event: %s", output)
	}
}

func TestLoggerDebugAtInfoLevel_Skipped(t *testing.T) {
	var buf bytes.Buffer
	l := New(false, "info", &buf)
	l.Debug("db.query", "SELECT 1")

	output := buf.String()
	if output != "" {
		t.Errorf("debug should be empty at info level: %s", output)
	}
}
