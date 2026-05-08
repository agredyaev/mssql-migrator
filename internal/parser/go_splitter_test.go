package parser

import "testing"

func TestSplitGO(t *testing.T) {
	batches, err := SplitGO("SELECT 1;\nGO\nSELECT 2;\nGO 2\nSELECT 'GO';")
	if err != nil {
		t.Fatal(err)
	}
	if len(batches) != 3 {
		t.Fatalf("expected 3 batches, got %d", len(batches))
	}
	if batches[1].Repeat != 2 {
		t.Fatalf("expected repeat 2, got %d", batches[1].Repeat)
	}
}

func TestSplitGOIgnoresInlineGO(t *testing.T) {
	batches, err := SplitGO("SELECT 'GO';")
	if err != nil {
		t.Fatal(err)
	}
	if len(batches) != 1 {
		t.Fatalf("expected 1 batch, got %d", len(batches))
	}
}
