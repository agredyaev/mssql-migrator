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
