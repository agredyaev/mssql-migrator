WITH inspector_column_scope AS (
    SELECT
        LOWER(JSON_VALUE(value, '$.schema')) AS schema_name,
        LOWER(JSON_VALUE(value, '$.object')) AS table_name
    FROM OPENJSON(@p1)
    WHERE LOWER(JSON_VALUE(value, '$.kind')) = 'tables'
)
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
JOIN inspector_column_scope sc ON sc.schema_name = LOWER(s.name)
    AND sc.table_name = LOWER(o.name)
WHERE o.is_ms_shipped = 0
  AND o.type = 'U'
