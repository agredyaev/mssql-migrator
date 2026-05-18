WITH inspector_object_schema_filter AS (
    SELECT DISTINCT CONVERT(nvarchar(128), [value]) AS schema_name
    FROM OPENJSON(@p1)
),
inspector_object_name_filter AS (
    SELECT DISTINCT CONVERT(nvarchar(128), [value]) AS object_name
    FROM OPENJSON(@p2)
)
SELECT
    LOWER(s.name) AS schema_name,
    CASE LOWER(o.type_desc)
        WHEN 'user_table' THEN 'tables'
        WHEN 'view' THEN 'views'
        WHEN 'sql_stored_procedure' THEN 'procedures'
        WHEN 'sql_scalar_function' THEN 'functions'
        WHEN 'sql_table_valued_function' THEN 'functions'
        WHEN 'sql_inline_table_valued_function' THEN 'functions'
        WHEN 'sql_trigger' THEN 'triggers'
        WHEN 'service_queue' THEN 'queues'
        WHEN 'synonym' THEN 'synonyms'
        WHEN 'sequence_object' THEN 'sequences'
        ELSE LOWER(o.type_desc)
    END AS kind,
    LOWER(o.name) AS object_name,
    LOWER(ISNULL(parent.name, '')) AS parent_name
FROM sys.objects o
JOIN sys.schemas s ON s.schema_id = o.schema_id
JOIN inspector_object_schema_filter sf ON sf.schema_name = LOWER(s.name)
JOIN inspector_object_name_filter nf ON nf.object_name = LOWER(o.name)
LEFT JOIN sys.objects parent ON parent.object_id = o.parent_object_id
WHERE o.is_ms_shipped = 0

UNION ALL

SELECT
    LOWER(s.name) AS schema_name,
    'types' AS kind,
    LOWER(tt.name) AS object_name,
    '' AS parent_name
FROM sys.table_types tt
JOIN sys.schemas s ON s.schema_id = tt.schema_id
JOIN inspector_object_schema_filter sf ON sf.schema_name = LOWER(s.name)
JOIN inspector_object_name_filter nf ON nf.object_name = LOWER(tt.name)

UNION ALL

SELECT
    LOWER(s.name) AS schema_name,
    'indexes' AS kind,
    LOWER(i.name) AS object_name,
    LOWER(o.name) AS parent_name
FROM sys.indexes i
JOIN sys.objects o ON o.object_id = i.object_id
JOIN sys.schemas s ON s.schema_id = o.schema_id
JOIN inspector_object_schema_filter sf ON sf.schema_name = LOWER(s.name)
JOIN inspector_object_name_filter nf ON nf.object_name = LOWER(i.name)
WHERE i.is_hypothetical = 0
  AND i.name IS NOT NULL
  AND o.type IN ('U', 'V')
  AND o.is_ms_shipped = 0
