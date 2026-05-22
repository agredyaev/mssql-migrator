WITH inspector_scope AS (
    SELECT
        LOWER(JSON_VALUE(value, '$.schema')) AS schema_name,
        LOWER(JSON_VALUE(value, '$.kind')) AS kind,
        LOWER(JSON_VALUE(value, '$.object')) AS object_name
    FROM OPENJSON(@p1)
)
SELECT TOP (1) CAST(1 AS int) AS hit
FROM sys.objects o
INNER JOIN sys.schemas s ON s.schema_id = o.schema_id
INNER JOIN inspector_scope sc ON sc.schema_name = LOWER(s.name)
    AND sc.object_name = LOWER(o.name)
WHERE o.is_ms_shipped = 0
