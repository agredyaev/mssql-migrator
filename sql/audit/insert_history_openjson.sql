INSERT INTO azdo_deploy_meta.history (normalized_key, kind, checksum, git_hash, git_author, git_date, event, error_text, created_at)
SELECT
    rows.normalized_key,
    rows.kind,
    rows.checksum,
    rows.git_hash,
    rows.git_author,
    COALESCE(TRY_CONVERT(datetime2, NULLIF(rows.git_date, ''), 127), CONVERT(datetime2, '1900-01-01T00:00:00')),
    rows.event,
    NULLIF(rows.error_text, ''),
    SYSUTCDATETIME()
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
) AS rows;
