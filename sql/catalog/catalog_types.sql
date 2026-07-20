SELECT
    CAST('object' AS varchar(8)) AS row_kind,
    LOWER(s.name) AS schema_name,
    CAST('types' AS varchar(32)) AS kind,
    LOWER(tt.name) AS object_name,
    CAST('' AS nvarchar(128)) AS parent_name
FROM sys.table_types tt
INNER JOIN sys.schemas s ON s.schema_id = tt.schema_id
INNER JOIN inspector_scope sc ON sc.schema_name = LOWER(s.name) COLLATE DATABASE_DEFAULT
    AND sc.object_name = LOWER(tt.name) COLLATE DATABASE_DEFAULT
    AND sc.kind = 'types'
UNION ALL
SELECT
    CAST('object' AS varchar(8)) AS row_kind,
    LOWER(s.name) AS schema_name,
    CAST('types' AS varchar(32)) AS kind,
    LOWER(t.name) AS object_name,
    CAST('' AS nvarchar(128)) AS parent_name
FROM sys.types t
INNER JOIN sys.schemas s ON s.schema_id = t.schema_id
INNER JOIN inspector_scope sc ON sc.schema_name = LOWER(s.name) COLLATE DATABASE_DEFAULT
    AND sc.object_name = LOWER(t.name) COLLATE DATABASE_DEFAULT
    AND sc.kind = 'types'
WHERE t.is_user_defined = 1
  AND t.is_table_type = 0
