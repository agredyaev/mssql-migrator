package validate

import "testing"

func TestBracketEscapesClosingBracket(t *testing.T) {
	if bracket("a]b") != "[a]]b]" {
		t.Fatal("bad bracket")
	}
}
