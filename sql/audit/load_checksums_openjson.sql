DECLARE @has_live_definition_checksum bit =
    CASE WHEN COL_LENGTH('azdo_deploy_meta.history', 'live_definition_checksum') IS NULL THEN 0 ELSE 1 END;

DECLARE @sql nvarchar(max) = N'
WITH checksum_keys AS (
    SELECT DISTINCT CONVERT(nvarchar(512), [value]) COLLATE DATABASE_DEFAULT AS normalized_key
    FROM OPENJSON(@p1)
), latest AS (
    SELECT h2.normalized_key, MAX(h2.id) AS max_id
    FROM azdo_deploy_meta.history h2
    JOIN checksum_keys k ON k.normalized_key = h2.normalized_key COLLATE DATABASE_DEFAULT
    WHERE h2.kind = ''object'' AND h2.event IN (''applied'', ''adopted'')
    GROUP BY h2.normalized_key
), rows AS (
    SELECT h.normalized_key, h.checksum, ' +
    CASE WHEN @has_live_definition_checksum = 1 THEN N'h.live_definition_checksum' ELSE N'CAST(NULL AS varbinary(32))' END + N' AS live_definition_checksum,
        LEFT(h.normalized_key, CHARINDEX(''/'', h.normalized_key) - 1) AS schema_name,
        SUBSTRING(h.normalized_key, CHARINDEX(''/'', h.normalized_key) + 1,
            CHARINDEX(''/'', h.normalized_key, CHARINDEX(''/'', h.normalized_key) + 1) - CHARINDEX(''/'', h.normalized_key) - 1) AS object_kind,
        SUBSTRING(h.normalized_key, CHARINDEX(''/'', h.normalized_key, CHARINDEX(''/'', h.normalized_key) + 1) + 1, 512) AS object_name
    FROM azdo_deploy_meta.history h
    JOIN latest ON h.normalized_key COLLATE DATABASE_DEFAULT = latest.normalized_key AND h.id = latest.max_id
)
SELECT rows.normalized_key, rows.checksum,
    CASE WHEN rows.object_kind NOT IN (''views'', ''functions'', ''procedures'', ''triggers'') THEN 0
         WHEN rows.checksum IS NULL OR LEN(rows.checksum) <> 64 THEN 0
         WHEN rows.live_definition_checksum IS NULL THEN 1
         WHEN live.definition_checksum IS NULL THEN 1
         WHEN rows.live_definition_checksum <> live.definition_checksum THEN 1
         ELSE 0 END AS live_definition_drift
FROM rows
OUTER APPLY (
    SELECT HASHBYTES(''SHA2_256'', OBJECT_DEFINITION(o.object_id)) AS definition_checksum
    FROM sys.objects AS o
    INNER JOIN sys.schemas AS s ON s.schema_id = o.schema_id
    WHERE LOWER(s.name) = rows.schema_name COLLATE DATABASE_DEFAULT
      AND LOWER(o.name) = rows.object_name COLLATE DATABASE_DEFAULT
      AND ((rows.object_kind = ''views'' AND o.type = ''V'')
        OR (rows.object_kind = ''functions'' AND o.type IN (''FN'', ''IF'', ''TF'', ''FS'', ''FT''))
        OR (rows.object_kind = ''procedures'' AND o.type IN (''P'', ''PC''))
        OR (rows.object_kind = ''triggers'' AND o.type = ''TR''))
) AS live;';

EXEC sp_executesql @sql, N'@p1 nvarchar(max)', @p1 = @p1;
