package logger

import (
	"bytes"
	"encoding/json"
	"strings"
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
