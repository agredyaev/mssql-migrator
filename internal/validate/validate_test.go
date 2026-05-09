package validate

import (
	"testing"

	"reporting-db-migrations/internal/catalog"
	"reporting-db-migrations/internal/parser"
)

func TestBracketEscapesClosingBracket(t *testing.T) {
	if bracket("a]b") != "[a]]b]" {
		t.Fatal("bad bracket")
	}
}

func TestManagedScopeRefsUsesPlannerNormalizedKeys(t *testing.T) {
	expected := []parser.Object{{Path: "reporting/triggers/snapshot/trg_audit.sql", NormalizedKey: "reporting/triggers/snapshot/trg_audit"}, {Path: "reporting/indexes/snapshot/ix_snapshot.sql", NormalizedKey: "reporting/indexes/snapshot/ix_snapshot"}}
	actual := map[string]CatalogObject{
		"reporting/triggers/snapshot/trg_audit":  {SchemaName: "Reporting", Kind: "triggers", ObjectName: "trg_audit", ParentName: "snapshot"},
		"reporting/indexes/snapshot/ix_snapshot": {SchemaName: "Reporting", Kind: "indexes", ObjectName: "ix_snapshot", ParentName: "snapshot"},
	}

	refs, missing := managedScopeRefs(expected, actual)
	if len(missing) != 0 {
		t.Fatalf("unexpected missing refs: %#v", missing)
	}
	if refs["reporting/triggers/snapshot/trg_audit"].Kind != "triggers" {
		t.Fatalf("unexpected trigger ref: %#v", refs)
	}
	if refs["reporting/indexes/snapshot/ix_snapshot"].Kind != "indexes" {
		t.Fatalf("unexpected index ref: %#v", refs)
	}
}

func TestManagedScopeRefsReportsMissingObjects(t *testing.T) {
	expected := []parser.Object{{Path: "reporting/views/monthly.sql", NormalizedKey: "reporting/views/monthly"}}
	refs, missing := managedScopeRefs(expected, map[string]CatalogObject{})
	if len(refs) != 0 {
		t.Fatalf("expected no refs, got %#v", refs)
	}
	if len(missing) != 1 || missing[0] != "reporting/views/monthly.sql" {
		t.Fatalf("unexpected missing refs: %#v", missing)
	}
}

func TestMapTypeDescToKind(t *testing.T) {
	tests := map[string]string{
		"USER_TABLE":                       "tables",
		"VIEW":                             "views",
		"SQL_STORED_PROCEDURE":             "procedures",
		"SQL_SCALAR_FUNCTION":              "functions",
		"SQL_INLINE_TABLE_VALUED_FUNCTION": "functions",
		"SQL_TABLE_VALUED_FUNCTION":        "functions",
		"SQL_TRIGGER":                      "triggers",
		"INDEX":                            "indexes",
		"USER_TABLE_TYPE":                  "types",
		"SEQUENCE_OBJECT":                  "sequences",
		"SYNONYM":                          "synonyms",
		"UNKNOWN":                          "",
	}

	for input, want := range tests {
		if got := catalog.MapTypeDescToKind(input); got != want {
			t.Fatalf("MapTypeDescToKind(%q) = %q, want %q", input, got, want)
		}
	}
}
