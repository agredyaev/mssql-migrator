// Package buildinfo holds compile-time version metadata (-ldflags -X) and optional VCS fallback.
package buildinfo

import (
	"encoding/json"
	"fmt"
	"io"
	"runtime/debug"
	"strings"
)

// Version is the semantic version string; overridden at link time via -ldflags -X.
var Version = "0.0.0-dev"

// Commit is a short source revision; overridden at link time via -ldflags -X.
var Commit = "unknown"

// Summary returns a single-line human-readable version and commit for stdout.
func Summary() string {
	return fmt.Sprintf("rmig %s %s", effectiveVersion(), effectiveCommit())
}

// WriteJSON writes one JSON object with "version" and "commit" keys.
func WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(map[string]string{
		"version": effectiveVersion(),
		"commit":  effectiveCommit(),
	})
}

func effectiveVersion() string {
	v := strings.TrimSpace(Version)
	if v == "" {
		return "0.0.0-dev"
	}
	return v
}

func effectiveCommit() string {
	c := strings.TrimSpace(Commit)
	if c != "" && c != "unknown" {
		return shortRev(c)
	}
	if rev := vcsRevisionFromBuildInfo(); rev != "" {
		return shortRev(rev)
	}
	return "unknown"
}

func shortRev(s string) string {
	s = strings.TrimPrefix(strings.TrimSpace(s), "vcs:")
	if len(s) <= 7 {
		return s
	}
	return s[:7]
}

func vcsRevisionFromBuildInfo() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, s := range bi.Settings {
		if s.Key == "vcs.revision" && s.Value != "" {
			return s.Value
		}
	}
	return ""
}
