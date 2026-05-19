package plan

import (
	"os"
	"testing"

	"reporting-db-migrations/internal/fs"
	"reporting-db-migrations/internal/types"
)

func TestBuildInspectScope_AllStableSkipsFullInspect(t *testing.T) {
	layout := fs.Layout{
		Objects: []*fs.Object{
			{
				SchemaName:    "r",
				Kind:          "views",
				ObjectName:    "v1",
				NormalizedKey: types.NormalizedKey("r", "views", "v1"),
				File:          &fs.CachedFile{AbsPath: t.TempDir() + "/v1.sql"},
			},
		},
	}
	path := layout.Objects[0].File.AbsPath
	if err := osWrite(path, "CREATE VIEW r.v1 AS SELECT 1\n"); err != nil {
		t.Fatal(err)
	}
	fileCS, err := layout.Objects[0].Checksum()
	if err != nil {
		t.Fatal(err)
	}
	checksums := map[string][32]byte{
		layout.Objects[0].NormalizedKey: fileCS, // file matches audit history
	}
	scope := BuildInspectScope(layout, nil, false, checksums)
	if scope.FullInspect {
		t.Fatal("expected scoped inspect when file matches history")
	}
	if len(scope.HotRefs) != 0 {
		t.Fatalf("expected 0 hot refs, got %d", len(scope.HotRefs))
	}
	if len(scope.StableObjects) != 1 {
		t.Fatalf("expected 1 stable object, got %d", len(scope.StableObjects))
	}
}

func TestBuildInspectScope_SpotCheckPromotesHot(t *testing.T) {
	t.Setenv("RMIG_CATALOG_SPOTCHECK", "1")
	layout := fs.Layout{
		Objects: []*fs.Object{
			{
				SchemaName:    "r",
				Kind:          "views",
				ObjectName:    "v1",
				NormalizedKey: types.NormalizedKey("r", "views", "v1"),
				File:          &fs.CachedFile{AbsPath: t.TempDir() + "/v1.sql"},
			},
			{
				SchemaName:    "r",
				Kind:          "views",
				ObjectName:    "v2",
				NormalizedKey: types.NormalizedKey("r", "views", "v2"),
				File:          &fs.CachedFile{AbsPath: t.TempDir() + "/v2.sql"},
			},
		},
	}
	for _, obj := range layout.Objects {
		if err := osWrite(obj.File.AbsPath, "CREATE VIEW r.v AS SELECT 1\n"); err != nil {
			t.Fatal(err)
		}
	}
	checksums := map[string][32]byte{}
	for _, obj := range layout.Objects {
		cs, err := obj.Checksum()
		if err != nil {
			t.Fatal(err)
		}
		checksums[obj.NormalizedKey] = cs
	}
	scope := BuildInspectScope(layout, nil, false, checksums)
	if len(scope.HotRefs) != 1 {
		t.Fatalf("expected 1 spot-check hot ref, got %d", len(scope.HotRefs))
	}
	if len(scope.StableObjects) != 1 {
		t.Fatalf("expected 1 stable object, got %d", len(scope.StableObjects))
	}
}

func osWrite(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}
