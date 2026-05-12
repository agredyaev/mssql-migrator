package checksum

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeSQL(t *testing.T) {
	input := "SELECT 1;   \r\nGO\t \rEND\t "
	want := "SELECT 1;\nGO\nEND"
	if got := NormalizeSQL(input); got != want {
		t.Fatalf("NormalizeSQL() = %q, want %q", got, want)
	}
	if SHA256String(input) != SHA256String(want) {
		t.Fatal("expected normalized checksums to match")
	}
	if SHA256String("SELECT 1;\t ") != SHA256String("SELECT 1;") {
		t.Fatal("expected eof trailing whitespace to be ignored")
	}
	if SHA256String("SELECT 1;\rGO") != SHA256String("SELECT 1;\nGO") {
		t.Fatal("expected carriage returns to normalize to line feeds")
	}
	if SHA256String("SELECT 1;\nGO\t \n") != SHA256String("SELECT 1;\nGO\n") {
		t.Fatal("expected trailing whitespace before newlines to be ignored")
	}
}

func TestSHA256StringMatchesNormalizedDigest(t *testing.T) {
	input := "SELECT 1;   \r\nGO\t \rEND\t "
	want := sha256.Sum256([]byte(NormalizeSQL(input)))
	if got := SHA256String(input); got != hex.EncodeToString(want[:]) {
		t.Fatalf("SHA256String() = %q, want %q", got, hex.EncodeToString(want[:]))
	}
	smallWant := sha256.Sum256([]byte("SELECT 1;"))
	if got := SHA256String("SELECT 1;"); got != hex.EncodeToString(smallWant[:]) {
		t.Fatalf("SHA256String() small input = %q", got)
	}
}

func TestSHA256FileMatchesSHA256StringAcrossBufferBoundaries(t *testing.T) {
	content := strings.Repeat("a", normalizeSQLBufferSize-1) + "\r\n" +
		strings.Repeat("b", normalizeSQLBufferSize-1) + " \n" +
		strings.Repeat("c", normalizeSQLBufferSize-1) + "\t "
	path := filepath.Join(t.TempDir(), "large.sql")
	writeChecksumFile(t, path, content)

	got, err := SHA256File(path)
	if err != nil {
		t.Fatal(err)
	}
	want := SHA256String(content)
	if got != want {
		t.Fatalf("SHA256File() = %q, want %q", got, want)
	}
}

func TestSQLDirHashNormalizesEquivalentFiles(t *testing.T) {
	left := t.TempDir()
	right := t.TempDir()

	writeChecksumFile(t, filepath.Join(left, "reporting", "views", "monthly.sql"), "SELECT 1;   \r\nGO\t \r\n")
	writeChecksumFile(t, filepath.Join(left, "reporting", "procedures", "refresh.sql"), "EXEC reporting.refresh   \r")
	writeChecksumFile(t, filepath.Join(left, "reporting", "views", "ignore.txt"), "ignored")

	writeChecksumFile(t, filepath.Join(right, "reporting", "views", "monthly.sql"), "SELECT 1;\nGO\n")
	writeChecksumFile(t, filepath.Join(right, "reporting", "procedures", "refresh.sql"), "EXEC reporting.refresh\n")
	writeChecksumFile(t, filepath.Join(right, "reporting", "views", "ignore.txt"), "different")

	leftHash, err := SQLDirHash(left)
	if err != nil {
		t.Fatal(err)
	}
	rightHash, err := SQLDirHash(right)
	if err != nil {
		t.Fatal(err)
	}
	if leftHash != rightHash {
		t.Fatalf("SQLDirHash() = %q, want %q", leftHash, rightHash)
	}
}

func writeChecksumFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir checksum file: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write checksum file: %v", err)
	}
}
