INSERT INTO azdo_deploy_meta.catalog_cache (normalized_key, schema_name, kind, object_name, parent_name, layout_digest)
SELECT j.normalized_key,
       j.schema_name,
       j.kind,
       j.object_name,
       ISNULL(j.parent_name, N''),
       @p2
FROM OPENJSON(@p1) WITH (
    normalized_key NVARCHAR(512) '$.k',
    schema_name    NVARCHAR(128) '$.s',
    kind           VARCHAR(32) '$.g',
    object_name    NVARCHAR(256) '$.o',
    parent_name    NVARCHAR(256) '$.p'
) j;
