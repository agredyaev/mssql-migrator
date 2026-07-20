CREATE TABLE #history_rows (
    normalized_key nvarchar(512) NOT NULL,
    checksum varchar(64) NULL,
    live_definition_checksum varbinary(32) NULL,
    applied_ddl_version bigint NULL
);

DECLARE @history_sql nvarchar(max) = N'
WITH checksum_keys AS (
    SELECT DISTINCT CONVERT(nvarchar(512), [value]) COLLATE DATABASE_DEFAULT AS normalized_key
    FROM OPENJSON(@p1)
), latest AS (
    SELECT h2.normalized_key, MAX(h2.id) AS max_id
    FROM azdo_deploy_meta.history h2
    JOIN checksum_keys k ON k.normalized_key = h2.normalized_key COLLATE DATABASE_DEFAULT
    WHERE h2.kind = ''object'' AND h2.event IN (''applied'', ''adopted'')
    GROUP BY h2.normalized_key
)
SELECT h.normalized_key, h.checksum, ' +
    CASE WHEN COL_LENGTH('azdo_deploy_meta.history', 'live_definition_checksum') IS NULL
        THEN N'CAST(NULL AS varbinary(32))'
        ELSE N'h.live_definition_checksum'
    END + N', ' +
    CASE WHEN COL_LENGTH('azdo_deploy_meta.history', 'applied_ddl_version') IS NULL
        THEN N'CAST(NULL AS bigint)'
        ELSE N'h.applied_ddl_version'
    END + N'
FROM azdo_deploy_meta.history h
JOIN latest
  ON h.normalized_key COLLATE DATABASE_DEFAULT = latest.normalized_key
 AND h.id = latest.max_id;';

INSERT INTO #history_rows (normalized_key, checksum, live_definition_checksum, applied_ddl_version)
EXEC sp_executesql @history_sql, N'@p1 nvarchar(max)', @p1 = @p1;

-- Incremental drift: only objects whose live definition may have changed since
-- the migrator last applied them ("suspects") get the expensive canonical
-- fingerprint below. If the DDL trigger is absent or disabled we cannot trust
-- object_ddl, so force every object to be a suspect (full-fingerprint fallback).
DECLARE @force_full bit = CASE WHEN EXISTS (
    SELECT 1 FROM sys.triggers
    WHERE parent_class = 0 AND name = 'azdo_deploy_meta_ddl_watch' AND is_disabled = 0
) THEN 0 ELSE 1 END;

WITH rows0 AS (
    SELECT h.normalized_key, h.checksum, h.live_definition_checksum, h.applied_ddl_version,
        CAST('object' AS varchar(16)) AS kind,
        LEFT(h.normalized_key, CHARINDEX('/', h.normalized_key) - 1) AS schema_name,
        SUBSTRING(h.normalized_key, CHARINDEX('/', h.normalized_key) + 1,
            CHARINDEX('/', h.normalized_key, CHARINDEX('/', h.normalized_key) + 1)
                - CHARINDEX('/', h.normalized_key) - 1) AS object_kind,
        SUBSTRING(h.normalized_key,
            CHARINDEX('/', h.normalized_key, CHARINDEX('/', h.normalized_key) + 1) + 1,
            512) AS object_name
    FROM #history_rows AS h
), rows AS (
    SELECT r.normalized_key, r.checksum, r.live_definition_checksum, r.kind,
        r.schema_name, r.object_kind, r.object_name,
        CASE WHEN @force_full = 1 THEN CAST(1 AS bit)
             WHEN r.applied_ddl_version IS NULL THEN CAST(1 AS bit)
             WHEN ISNULL(od.ddl_version, 0) > r.applied_ddl_version THEN CAST(1 AS bit)
             ELSE CAST(0 AS bit) END AS is_suspect
    FROM rows0 AS r
    LEFT JOIN azdo_deploy_meta.object_ddl AS od
        ON od.schema_name = r.schema_name AND od.object_name = r.object_name
)
SELECT rows.normalized_key, rows.checksum,
    CASE WHEN rows.object_kind NOT IN (
            'types', 'sequences', 'tables', 'synonyms', 'indexes',
            'views', 'functions', 'procedures', 'triggers'
         ) THEN 0
         WHEN rows.checksum IS NULL OR LEN(rows.checksum) <> 64 THEN 0
         -- No stored fingerprint (legacy/never captured) means no baseline to
         -- compare against, so it must be treated as drift regardless of the
         -- suspect fast-path below.
         WHEN rows.live_definition_checksum IS NULL THEN 1
         WHEN rows.is_suspect = 0 THEN 0
         WHEN live.definition_checksum IS NULL THEN 1
         WHEN rows.live_definition_checksum <> live.definition_checksum THEN 1
         ELSE 0 END AS live_definition_drift
FROM rows
