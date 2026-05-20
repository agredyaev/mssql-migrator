package fs

import "testing"

func TestObjectKindCode(t *testing.T) {
	if ObjectKindCode("tables") != KindCodeTables {
		t.Fatalf("tables code")
	}
	if ObjectKindCode("unknown") != KindCodeUnknown {
		t.Fatalf("unknown code")
	}
}

func TestIsModuleKindCode(t *testing.T) {
	if !IsModuleKindCode(KindCodeViews) {
		t.Fatal("views module")
	}
	if IsModuleKindCode(KindCodeTables) {
		t.Fatal("tables not module")
	}
}
