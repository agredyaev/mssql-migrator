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
LEFT JOIN sys.objects parent ON parent.object_id = o.parent_object_id
WHERE o.is_ms_shipped = 0
  AND LOWER(s.name) IN ({{schema_list}})
  AND LOWER(o.name) IN ({{object_list}})

UNION ALL

SELECT
    LOWER(s.name) AS schema_name,
    'types' AS kind,
    LOWER(tt.name) AS object_name,
    '' AS parent_name
FROM sys.table_types tt
JOIN sys.schemas s ON s.schema_id = tt.schema_id
WHERE LOWER(s.name) IN ({{schema_list}})
  AND LOWER(tt.name) IN ({{object_list}})

UNION ALL

SELECT
    LOWER(s.name) AS schema_name,
    'indexes' AS kind,
    LOWER(i.name) AS object_name,
    LOWER(o.name) AS parent_name
FROM sys.indexes i
JOIN sys.objects o ON o.object_id = i.object_id
JOIN sys.schemas s ON s.schema_id = o.schema_id
WHERE i.is_hypothetical = 0
  AND i.name IS NOT NULL
  AND o.type IN ('U', 'V')
  AND o.is_ms_shipped = 0
  AND LOWER(s.name) IN ({{schema_list}})
  AND LOWER(i.name) IN ({{object_list}})
