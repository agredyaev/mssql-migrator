package db

import (
	"os"
	"testing"

	"reporting-db-migrations/internal/fs"
)

func TestCatalogCacheEnabled(t *testing.T) {
	t.Setenv("RMIG_CATALOG_CACHE", "")
	t.Setenv("RMIG_INSPECT_FULL", "")
	if !CatalogCacheEnabled() {
		t.Fatal("expected enabled by default")
	}
	t.Setenv("RMIG_CATALOG_CACHE", "0")
	if CatalogCacheEnabled() {
		t.Fatal("expected disabled when RMIG_CATALOG_CACHE=0")
	}
}

func TestMarshalCatalogCacheJSON_Count(t *testing.T) {
	objects := map[string]Object{
		"k1": {SchemaName: "r", Kind: "tables", ObjectName: "t"},
	}
	s, err := marshalCatalogCacheJSON(objects, 1)
	if err != nil {
		t.Fatal(err)
	}
	if s == "" {
		t.Fatal("expected JSON payload")
	}
	_, err = marshalCatalogCacheJSON(objects, 2)
	if err != nil {
		t.Fatal(err)
	}
}

func TestLayoutDigestStable(t *testing.T) {
	layout := fs.Layout{
		Objects: []*fs.Object{{NormalizedKey: "r/tables/t1", SchemaName: "r", Kind: "tables", ObjectName: "t1"}},
	}
	a := LayoutDigest(layout)
	b := LayoutDigest(layout)
	if a != b || a == "" {
		t.Fatalf("digest=%q want stable non-empty", a)
	}
}

func TestSpotCheckCountFromEnv(t *testing.T) {
	os.Unsetenv("RMIG_CATALOG_SPOTCHECK")
	if SpotCheckCountFromEnv() != 0 {
		t.Fatal("expected 0")
	}
	t.Setenv("RMIG_CATALOG_SPOTCHECK", "3")
	if SpotCheckCountFromEnv() != 3 {
		t.Fatal("expected 3")
	}
}
