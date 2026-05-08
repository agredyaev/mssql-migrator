package app

import (
	"bytes"
	"reporting-db-migrations/internal/contracts"
	"strings"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	stdout := bytes.Buffer{}
	runtime := Runtime{BuildInfo: BuildInfo{Version: "4.0.0", Commit: "abc"}, Stdout: &stdout}
	code := runtime.Run([]string{"reporting-migrator", "version"})
	if code != contracts.ExitOK {
		t.Fatalf("unexpected exit code: %d", code)
	}
	if stdout.String() != "reporting-migrator 4.0.0 commit=abc\n" {
		t.Fatalf("unexpected output: %s", stdout.String())
	}
}

func TestUsageIncludesV11Commands(t *testing.T) {
	buffer := bytes.Buffer{}
	printUsage(&buffer)
	output := buffer.String()
	for _, expected := range []string{"baseline --env prod --up-to V010 --confirm", "repair-checksum --env prod --script R002__views.sql --confirm"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("usage missing %q: %s", expected, output)
		}
	}
}
