package logger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestRedactMasksKnownSecretPatterns(t *testing.T) {
	input := "server=x;password=secret;pwd=short;token=abc;access_token=xyz;client_secret=c"
	got := Redact(input)
	for _, forbidden := range []string{"password=secret", "pwd=short", "token=abc", "access_token=xyz", "client_secret=c"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("redaction leaked %q in %q", forbidden, got)
		}
	}
}

func TestJSONLoggerWritesValidJSON(t *testing.T) {
	buffer := bytes.Buffer{}
	log := New(Options{JSON: true, Level: "debug", Writer: &buffer})
	log.Info("test_event", "password=secret")
	payload := map[string]string{}
	if err := json.Unmarshal(buffer.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json log: %v", err)
	}
	if payload["event"] != "test_event" {
		t.Fatalf("unexpected event: %s", payload["event"])
	}
	if strings.Contains(payload["message"], "secret") {
		t.Fatalf("secret leaked in json log: %s", payload["message"])
	}
}

func TestRedactMasksSensitiveURLQueryValues(t *testing.T) {
	input := "https://ci.example/run?token=abc123&sig=xyz987&ok=1"
	got := Redact(input)
	for _, forbidden := range []string{"token=abc123", "sig=xyz987"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("redaction leaked %q in %q", forbidden, got)
		}
	}
}

func TestRedactMasksQuotedAndSpacedSecrets(t *testing.T) {
	input := "password='my secret'; token=\"abc 123\"; client_secret=top secret"
	got := Redact(input)
	for _, forbidden := range []string{"my secret", "abc 123", "top secret"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("redaction leaked %q in %q", forbidden, got)
		}
	}
}

func TestRedactMasksJSONSecrets(t *testing.T) {
	input := `{"password":"secret","access_token":"abc","safe":"ok"}`
	got := Redact(input)
	for _, forbidden := range []string{"secret", "abc"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("redaction leaked %q in %q", forbidden, got)
		}
	}
	if !strings.Contains(got, `"safe":"ok"`) {
		t.Fatalf("expected safe field to remain visible, got %q", got)
	}
}

func TestLoggerConcurrentWrites(t *testing.T) {
	buffer := bytes.Buffer{}
	log := New(Options{Level: "info", Writer: &buffer})
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			log.Info("concurrent", fmt.Sprintf("message=%d", i))
		}(i)
	}
	wg.Wait()
	lines := strings.Split(strings.TrimSpace(buffer.String()), "\n")
	if len(lines) != 32 {
		t.Fatalf("expected 32 log lines, got %d in %q", len(lines), buffer.String())
	}
}
