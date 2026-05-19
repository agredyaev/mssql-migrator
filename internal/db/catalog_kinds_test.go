package db

import (
	"strings"
	"testing"

	"reporting-db-migrations/internal/fs"
	"reporting-db-migrations/internal/types"
)

func TestBuildCatalogStateSQL_KindFilter(t *testing.T) {
	layout := fs.Layout{
		Objects: []*fs.Object{
			{SchemaName: "r", Kind: "tables", ObjectName: "t1", NormalizedKey: types.NormalizedKey("r", "tables", "t1")},
		},
	}
	sql := buildCatalogStateSQL(catalogKindsForLayout(layout))
	if strings.Contains(sql, "index_rows") {
		t.Fatal("expected no index_rows CTE for tables-only layout")
	}
	if strings.Contains(sql, "type_rows") {
		t.Fatal("expected no type_rows CTE for tables-only layout")
	}
	if !strings.Contains(sql, "sys_object_rows") {
		t.Fatal("expected sys_object_rows CTE")
	}
}

func TestCatalogKindsForLayout(t *testing.T) {
	layout := fs.Layout{
		Objects: []*fs.Object{
			{Kind: "indexes"},
			{Kind: "types"},
			{Kind: "views"},
		},
	}
	k := catalogKindsForLayout(layout)
	if !k.indexes || !k.types || !k.sysObjects {
		t.Fatalf("kinds = %+v", k)
	}
}
