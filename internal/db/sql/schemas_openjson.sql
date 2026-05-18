WITH inspector_schema_filter AS (
    SELECT DISTINCT CONVERT(nvarchar(128), [value]) AS schema_name
    FROM OPENJSON(@p1)
)
SELECT LOWER(s.name) AS schema_name
FROM sys.schemas s
JOIN inspector_schema_filter sf ON sf.schema_name = LOWER(s.name)
