package db

import (
	_ "embed"
	"strings"
)

//go:embed sql/catalog_scope_header.sql
var catalogScopeHeaderSQL string

//go:embed sql/catalog_schema_rows.sql
var catalogSchemaRowsSQL string

//go:embed sql/catalog_sys_objects.sql
var catalogSysObjectsSQL string

//go:embed sql/catalog_types.sql
var catalogTypesSQL string

//go:embed sql/catalog_indexes.sql
var catalogIndexesSQL string

func buildCatalogStateSQL(kinds catalogKinds) string {
	var b strings.Builder
	b.Grow(len(catalogScopeHeaderSQL) + 512)
	b.WriteString(catalogScopeHeaderSQL)
	b.WriteString(", schema_rows AS (")
	b.WriteString(catalogSchemaRowsSQL)
	b.WriteString(")")

	if kinds.sysObjects {
		b.WriteString(", sys_object_rows AS (")
		b.WriteString(catalogSysObjectsSQL)
		b.WriteString(")")
	}
	if kinds.types {
		b.WriteString(", type_rows AS (")
		b.WriteString(catalogTypesSQL)
		b.WriteString(")")
	}
	if kinds.indexes {
		b.WriteString(", index_rows AS (")
		b.WriteString(catalogIndexesSQL)
		b.WriteString(")")
	}

	b.WriteString(" SELECT row_kind, schema_name, kind, object_name, parent_name FROM schema_rows")
	if kinds.sysObjects {
		b.WriteString(" UNION ALL SELECT row_kind, schema_name, kind, object_name, parent_name FROM sys_object_rows")
	}
	if kinds.types {
		b.WriteString(" UNION ALL SELECT row_kind, schema_name, kind, object_name, parent_name FROM type_rows")
	}
	if kinds.indexes {
		b.WriteString(" UNION ALL SELECT row_kind, schema_name, kind, object_name, parent_name FROM index_rows")
	}
	return b.String()
}
