package fs

import (
	"testing"
)

func TestNormalizeSQL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "whitespace normalization",
			input: "SELECT 1;   \r\nGO\t \rEND\t ",
			want:  "SELECT 1;\nGO\nEND",
		},
		{
			name:  "trailing whitespace",
			input: "SELECT 1;   ",
			want:  "SELECT 1;",
		},
		{
			name:  "carriage returns",
			input: "SELECT 1;\rGO",
			want:  "SELECT 1;\nGO",
		},
		{
			name:  "trailing whitespace before newline",
			input: "SELECT 1;\nGO\t \n",
			want:  "SELECT 1;\nGO\n",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeSQL(tt.input)
			if got != tt.want {
				t.Errorf("normalizeSQL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeAndHashDeterministic(t *testing.T) {
	h1 := NormalizeAndHash("SELECT 1;\t ")
	h2 := NormalizeAndHash("SELECT 1;")
	if h1 != h2 {
		t.Errorf("normalized hashes should match: %x vs %x", h1, h2)
	}
	if h1 == [32]byte{} {
		t.Fatal("hash is empty")
	}
}
