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

func TestSplitGODoesNotSplitInsideBlockComment(t *testing.T) {
	batches, err := SplitGO("/*\nGO\n*/\nSELECT 1;")
	if err != nil {
		t.Fatal(err)
	}
	if len(batches) != 1 {
		t.Fatalf("expected one batch, got %d", len(batches))
	}
}

func TestSplitGODoesNotSplitInsideMultilineString(t *testing.T) {
	sql := "EXEC('SELECT 1\nGO\nSELECT 2')\nGO\nSELECT 3"
	batches, err := SplitGO(sql)
	if err != nil {
		t.Fatal(err)
	}
	if len(batches) != 2 {
		t.Fatalf("expected 2 batches, got %d", len(batches))
	}
}

func TestSplitGODoesNotSplitInsideMultilineStringWithEscapedQuotes(t *testing.T) {
	sql := "EXEC('it''s line 1\nGO\nline 2')\nGO\nSELECT 3"
	batches, err := SplitGO(sql)
	if err != nil {
		t.Fatal(err)
	}
	if len(batches) != 2 {
		t.Fatalf("expected 2 batches, got %d", len(batches))
	}
}

func TestSplitGOIgnoresGOInsideInlineComment(t *testing.T) {
	batches, err := SplitGO("SELECT 1; -- GO\nGO\n-- GO\nSELECT 2;")
	if err != nil {
		t.Fatal(err)
	}
	if len(batches) != 2 {
		t.Fatalf("expected 2 batches, got %d", len(batches))
	}
}

func TestSplitGOSkipsEmptyBatches(t *testing.T) {
	batches, err := SplitGO("GO\n\nGO\nSELECT 1;\nGO\nGO")
	if err != nil {
		t.Fatal(err)
	}
	if len(batches) != 1 {
		t.Fatalf("expected 1 batch, got %d", len(batches))
	}
}
