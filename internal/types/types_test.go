package types

import "testing"

func TestNormalizedKey(t *testing.T) {
	tests := []struct {
		name   string
		schema string
		kind   string
		obj    string
		want   string
	}{
		{name: "standard", schema: "reporting", kind: "views", obj: "monthly", want: "reporting/views/monthly"},
		{name: "uppercase", schema: "REPORTING", kind: "VIEWS", obj: "MONTHLY", want: "reporting/views/monthly"},
		{name: "mixed case", schema: "Reporting", kind: "Views", obj: "Monthly", want: "reporting/views/monthly"},
		{name: "empty kind", schema: "r", kind: "", obj: "v", want: "r//v"},
		{name: "all empty", schema: "", kind: "", obj: "", want: "//"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizedKey(tt.schema, tt.kind, tt.obj)
			if got != tt.want {
				t.Errorf("NormalizedKey(%q, %q, %q) = %q, want %q", tt.schema, tt.kind, tt.obj, got, tt.want)
			}
		})
	}
}

func TestNormalizedKeyDeterministic(t *testing.T) {
	key1 := NormalizedKey("Reporting", "Views", "Monthly")
	key2 := NormalizedKey("reporting", "views", "monthly")
	if key1 != key2 {
		t.Errorf("NormalizedKey not deterministic: %q != %q", key1, key2)
	}
}

func TestConfigDefaults(t *testing.T) {
	var cfg Config

	if got := cfg.DBAuthMode(); got != DBAuthSQL {
		t.Errorf("DBAuthMode() = %q, want %q", got, DBAuthSQL)
	}
	if got := cfg.EffectiveUpdatePolicy(); got != UpdatePolicyAllSupported {
		t.Errorf("EffectiveUpdatePolicy() = %q, want %q", got, UpdatePolicyAllSupported)
	}
	if got := cfg.EffectiveTransactionMode(); got != TransactionModeScript {
		t.Errorf("EffectiveTransactionMode() = %q, want %q", got, TransactionModeScript)
	}
}

func TestConfigExplicitValues(t *testing.T) {
	cfg := Config{
		DBAuth:          "integrated",
		UpdatePolicy:    "none",
		TransactionMode: "none",
	}

	if got := cfg.DBAuthMode(); got != "integrated" {
		t.Errorf("DBAuthMode() = %q, want integrated", got)
	}
	if got := cfg.EffectiveUpdatePolicy(); got != "none" {
		t.Errorf("EffectiveUpdatePolicy() = %q, want none", got)
	}
	if got := cfg.EffectiveTransactionMode(); got != "none" {
		t.Errorf("EffectiveTransactionMode() = %q, want none", got)
	}
}

func TestExitCodesAreDistinct(t *testing.T) {
	codes := map[int]string{
		ExitOK:               "ExitOK",
		ExitGeneralError:     "ExitGeneralError",
		ExitConfigError:      "ExitConfigError",
		ExitConnError:        "ExitConnError",
		ExitChecksumMismatch: "ExitChecksumMismatch",
		ExitSQLExecution:     "ExitSQLExecution",
		ExitValidation:       "ExitValidation",
		ExitLockTimeout:      "ExitLockTimeout",
		ExitInvalidInput:     "ExitInvalidInput",
		ExitCriticalState:    "ExitCriticalState",
	}
	seen := map[int]string{}
	for code, label := range codes {
		if _, ok := seen[code]; ok {
			t.Errorf("duplicate exit code %d: %s and %s", code, seen[code], label)
		}
		seen[code] = label
	}
	if len(seen) != len(codes) {
		t.Fatalf("exit code collision detected: %d unique vs %d declared", len(seen), len(codes))
	}
}

func TestChunkKeys_Empty(t *testing.T) {
	if got := ChunkKeys(nil, 2100); got != nil {
		t.Errorf("ChunkKeys(nil) = %v, want nil", got)
	}
	if got := ChunkKeys([]string{}, 2100); got != nil {
		t.Errorf("ChunkKeys([]) = %v, want nil", got)
	}
}

func TestChunkKeys_LargeBatch(t *testing.T) {
	keys := make([]string, 4200)
	for i := range keys {
		keys[i] = "k"
	}
	chunks := ChunkKeys(keys, 2100)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks for 4200 keys, got %d", len(chunks))
	}
	if len(chunks[0]) != 2100 {
		t.Errorf("first chunk = %d, want 2100", len(chunks[0]))
	}
	if len(chunks[1]) != 2100 {
		t.Errorf("second chunk = %d, want 2100", len(chunks[1]))
	}
}

func TestBuildINQuery(t *testing.T) {
	q, args := BuildINQuery("SELECT * FROM t WHERE c IN ({{list}})", "{{list}}", []string{"a", "b", "c"})
	if len(args) != 3 {
		t.Errorf("expected 3 args, got %d", len(args))
	}
	if args[0] != "a" || args[1] != "b" || args[2] != "c" {
		t.Errorf("unexpected args: %v", args)
	}
	if len(q) == 0 {
		t.Error("query is empty")
	}
}
