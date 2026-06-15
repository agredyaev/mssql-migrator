SELECT
    CAST('schema' AS varchar(8)) AS row_kind,
    LOWER(s.name) AS schema_name,
    CAST('' AS varchar(32)) AS kind,
    CAST('' AS nvarchar(128)) AS object_name,
    CAST('' AS nvarchar(128)) AS parent_name
FROM sys.schemas s
WHERE EXISTS (
    SELECT 1 FROM layout_schema_filter lf WHERE lf.schema_name = LOWER(s.name) COLLATE DATABASE_DEFAULT
)
