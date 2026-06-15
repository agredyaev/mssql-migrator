WITH inspector_scope AS (
    SELECT
        LOWER(JSON_VALUE(value, '$.schema')) COLLATE DATABASE_DEFAULT AS schema_name,
        LOWER(JSON_VALUE(value, '$.kind')) COLLATE DATABASE_DEFAULT AS kind,
        LOWER(JSON_VALUE(value, '$.object')) COLLATE DATABASE_DEFAULT AS object_name
    FROM OPENJSON(@p1)
),
layout_schema_filter AS (
    SELECT DISTINCT LOWER(CONVERT(nvarchar(128), [value])) COLLATE DATABASE_DEFAULT AS schema_name
    FROM OPENJSON(@p2)
)
