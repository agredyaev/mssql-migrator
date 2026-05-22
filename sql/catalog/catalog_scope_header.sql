WITH inspector_scope AS (
    SELECT
        LOWER(JSON_VALUE(value, '$.schema')) AS schema_name,
        LOWER(JSON_VALUE(value, '$.kind')) AS kind,
        LOWER(JSON_VALUE(value, '$.object')) AS object_name
    FROM OPENJSON(@p1)
),
layout_schema_filter AS (
    SELECT DISTINCT LOWER(CONVERT(nvarchar(128), [value])) AS schema_name
    FROM OPENJSON(@p2)
)
