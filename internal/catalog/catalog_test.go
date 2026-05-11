package catalog

import (
	"context"
	"errors"
	"strings"
	"testing"

	"reporting-db-migrations/internal/contracts"
)

func TestReadRejectsNilConnection(t *testing.T) {
	_, err := Read(context.Background(), nil)
	if err == nil {
		t.Fatal("expected nil connection error")
	}
	if !errors.Is(err, contracts.ErrCriticalState) {
		t.Fatalf("expected critical state sentinel, got %v", err)
	}
	if err.Error() == contracts.ErrCriticalState.Error() {
		t.Fatalf("expected descriptive wrapped error, got %v", err)
	}
}

func TestStateQueryForSchemasRepeatsArgsPerUnionBranch(t *testing.T) {
	query, args := stateQueryForSchemas([]string{"reporting", "sales", "reporting"})
	if got, want := len(args), 6; got != want {
		t.Fatalf("stateQueryForSchemas() args=%d, want %d", got, want)
	}
	for _, expected := range []string{
		"s.name IN (@p1, @p2)",
		"s.name IN (@p3, @p4)",
		"s.name IN (@p5, @p6)",
	} {
		if !strings.Contains(query, expected) {
			t.Fatalf("expected query to contain %q, got %s", expected, query)
		}
	}
	for i, expected := range []any{"reporting", "sales", "reporting", "sales", "reporting", "sales"} {
		if args[i] != expected {
			t.Fatalf("args[%d]=%v, want %v", i, args[i], expected)
		}
	}
}

func TestNormalizedSchemaFilterDeduplicatesNames(t *testing.T) {
	got := normalizedSchemaFilter([]string{"Reporting", " reporting ", "Sales", "sales", ""})
	if want := []string{"Reporting", "Sales"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("normalizedSchemaFilter()=%#v, want %#v", got, want)
	}
}
