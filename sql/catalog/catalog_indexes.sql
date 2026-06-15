SELECT
    CAST('object' AS varchar(8)) AS row_kind,
    LOWER(s.name) AS schema_name,
    CAST('indexes' AS varchar(32)) AS kind,
    LOWER(i.name) AS object_name,
    LOWER(o.name) AS parent_name
FROM sys.indexes i
INNER JOIN sys.objects o ON o.object_id = i.object_id
INNER JOIN sys.schemas s ON s.schema_id = o.schema_id
INNER JOIN inspector_scope sc ON sc.schema_name = LOWER(s.name) COLLATE DATABASE_DEFAULT
    AND sc.object_name = LOWER(i.name) COLLATE DATABASE_DEFAULT
    AND sc.kind = 'indexes'
WHERE i.is_hypothetical = 0
  AND i.name IS NOT NULL
  AND o.type IN ('U', 'V')
  AND o.is_ms_shipped = 0
