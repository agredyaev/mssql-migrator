package buildinfo

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestSummary_nonEmpty(t *testing.T) {
	s := Summary()
	if !strings.HasPrefix(s, "rmig ") {
		t.Fatalf("Summary() = %q, want prefix rmig ", s)
	}
}

func TestWriteJSON_roundTrip(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteJSON(&buf); err != nil {
		t.Fatal(err)
	}
	var m map[string]string
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, buf.String())
	}
	if m["version"] == "" {
		t.Fatal("missing version")
	}
	if m["commit"] == "" {
		t.Fatal("missing commit")
	}
}
