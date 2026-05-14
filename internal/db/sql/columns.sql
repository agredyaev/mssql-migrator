SELECT
    LOWER(s.name) AS schema_name,
    LOWER(o.name) AS table_name,
    LOWER(c.name) AS column_name,
    LOWER(t.name) AS type_name,
    c.max_length AS length,
    c.precision,
    c.scale,
    c.is_nullable
FROM sys.columns c
JOIN sys.objects o ON o.object_id = c.object_id
JOIN sys.schemas s ON s.schema_id = o.schema_id
JOIN sys.types t ON t.user_type_id = c.user_type_id
WHERE o.is_ms_shipped = 0
  AND o.type = 'U'
  AND LOWER(s.name) IN ({{schema_list}})
  AND LOWER(o.name) IN ({{table_list}})
