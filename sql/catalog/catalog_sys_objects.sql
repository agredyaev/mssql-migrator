SELECT
    CAST('object' AS varchar(8)) AS row_kind,
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
INNER JOIN sys.schemas s ON s.schema_id = o.schema_id
INNER JOIN inspector_scope sc ON sc.schema_name = LOWER(s.name) COLLATE DATABASE_DEFAULT
    AND sc.object_name = LOWER(o.name) COLLATE DATABASE_DEFAULT
    AND sc.kind IN (
        'tables', 'views', 'procedures', 'functions', 'triggers', 'queues', 'synonyms', 'sequences'
    )
    AND sc.kind = CASE LOWER(o.type_desc)
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
    END COLLATE DATABASE_DEFAULT
LEFT JOIN sys.objects parent ON parent.object_id = o.parent_object_id
WHERE o.is_ms_shipped = 0
