SELECT
    LOWER(s.name) AS schema_name,
    LOWER(i.name) AS index_name,
    LOWER(o.name) AS parent_name
FROM sys.indexes i
INNER JOIN sys.objects o ON o.object_id = i.object_id
INNER JOIN sys.schemas s ON s.schema_id = o.schema_id
WHERE i.is_hypothetical = 0
  AND i.name IS NOT NULL
  AND o.type IN ('U', 'V')
  AND o.is_ms_shipped = 0
