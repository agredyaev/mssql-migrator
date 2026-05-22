SELECT
    CAST('object' AS varchar(8)) AS row_kind,
    LOWER(s.name) AS schema_name,
    CAST('types' AS varchar(32)) AS kind,
    LOWER(tt.name) AS object_name,
    CAST('' AS nvarchar(128)) AS parent_name
FROM sys.table_types tt
INNER JOIN sys.schemas s ON s.schema_id = tt.schema_id
INNER JOIN inspector_scope sc ON sc.schema_name = LOWER(s.name)
    AND sc.object_name = LOWER(tt.name)
    AND sc.kind = 'types'
