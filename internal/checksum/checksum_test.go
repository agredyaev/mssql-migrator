package checksum

import "testing"

func TestNormalizeSQL(t *testing.T) {
	left := SHA256String("SELECT 1;   \r\nGO\r\n")
	right := SHA256String("SELECT 1;\nGO\n")
	if left != right {
		t.Fatal("expected normalized checksums to match")
	}
}
