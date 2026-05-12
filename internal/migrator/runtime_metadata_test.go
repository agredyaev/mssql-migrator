package migrator

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPlanArtifactHashNormalizesEquivalentFiles(t *testing.T) {
	rawPath := filepath.Join(t.TempDir(), "migration-plan.json")
	normalizedPath := filepath.Join(t.TempDir(), "normalized-plan.json")

	if err := os.WriteFile(rawPath, []byte("{\r\n  \"plan\": 1   \r\n}\t "), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(normalizedPath, []byte("{\n  \"plan\": 1\n}"), 0o644); err != nil {
		t.Fatal(err)
	}

	rawHash, err := planArtifactHash(rawPath)
	if err != nil {
		t.Fatal(err)
	}
	normalizedHash, err := planArtifactHash(normalizedPath)
	if err != nil {
		t.Fatal(err)
	}
	if rawHash != normalizedHash {
		t.Fatalf("planArtifactHash() = %q, want %q", rawHash, normalizedHash)
	}
}
