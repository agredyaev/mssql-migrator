DECLARE @prepared TABLE (
    normalized_key nvarchar(512) NOT NULL,
    kind varchar(16) NOT NULL,
    object_kind nvarchar(32) NULL,
    checksum varchar(64) NOT NULL,
    live_definition_checksum varbinary(32) NULL,
    git_hash varchar(64) NOT NULL,
    git_author nvarchar(256) NOT NULL,
    git_date datetime2 NOT NULL,
    event varchar(16) NOT NULL,
    error_text nvarchar(max) NULL
);

WITH input_rows AS (
    SELECT *
    FROM OPENJSON(@p1)
WITH (
    normalized_key nvarchar(512) '$.normalized_key',
    kind           nvarchar(16)  '$.kind',
    checksum       nvarchar(64)  '$.checksum',
    git_hash       nvarchar(64)  '$.git_hash',
    git_author     nvarchar(256) '$.git_author',
    git_date       nvarchar(64)  '$.git_date',
    event          nvarchar(16)  '$.event',
    error_text     nvarchar(max) '$.error_text'
)
), parts AS (
    SELECT rows.*,
        LEFT(normalized_key, CHARINDEX('/', normalized_key) - 1) AS schema_name,
        SUBSTRING(normalized_key, CHARINDEX('/', normalized_key) + 1,
            CHARINDEX('/', normalized_key, CHARINDEX('/', normalized_key) + 1) - CHARINDEX('/', normalized_key) - 1) AS object_kind,
        SUBSTRING(normalized_key, CHARINDEX('/', normalized_key, CHARINDEX('/', normalized_key) + 1) + 1, 512) AS object_name
    FROM input_rows AS rows
)
INSERT INTO @prepared (normalized_key, kind, object_kind, checksum, live_definition_checksum, git_hash, git_author, git_date, event, error_text)
SELECT rows.normalized_key, rows.kind, rows.object_kind, rows.checksum, live.definition_checksum,
    rows.git_hash, rows.git_author,
    COALESCE(TRY_CONVERT(datetime2, NULLIF(rows.git_date, ''), 127), CONVERT(datetime2, '1900-01-01T00:00:00')),
    rows.event, NULLIF(rows.error_text, '')
    FROM parts AS rows
