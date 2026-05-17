package types

import (
	"encoding/json"
	"testing"
)

func TestPlannedObjectJSONRoundtrip_NoGit(t *testing.T) {
	p := PlannedObject{
		ObjectRef: ObjectRef{
			NormalizedKey: "db/views/v1",
			ObjectPath:    "db/views/v1.sql",
			SchemaName:    "dbo",
			Kind:          "views",
			ObjectName:    "v1",
		},
		PlannedAction: ActionSkipUnchanged,
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var got PlannedObject
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Git != nil {
		t.Fatalf("expected nil Git, got %+v", got.Git)
	}
	if got.NormalizedKey != p.NormalizedKey || got.ObjectPath != p.ObjectPath {
		t.Fatalf("object ref mismatch: %+v", got.ObjectRef)
	}
}

func TestPlannedObjectJSONRoundtrip_WithGit(t *testing.T) {
	g := GitInfo{GitHash: "abc", GitAuthor: "a", GitDate: "2024-01-01T00:00:00Z"}
	p := PlannedObject{
		ObjectRef: ObjectRef{NormalizedKey: "k", ObjectPath: "p.sql"},
		Git:       &g,
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var got PlannedObject
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Git == nil || got.Git.GitHash != "abc" || got.Git.GitAuthor != "a" || got.Git.GitDate != "2024-01-01T00:00:00Z" {
		t.Fatalf("git mismatch: %+v", got.Git)
	}
}

func TestPlannedObjectGitStrings(t *testing.T) {
	var p PlannedObject
	h, a, d := p.GitStrings()
	if h != "" || a != "" || d != "" {
		t.Fatalf("expected empty strings, got %q %q %q", h, a, d)
	}
	p.Git = &GitInfo{GitHash: "h"}
	h, _, _ = p.GitStrings()
	if h != "h" {
		t.Fatalf("got %q", h)
	}
}
