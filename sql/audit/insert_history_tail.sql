
IF EXISTS (
    SELECT 1 FROM @prepared
    WHERE kind = 'object' AND event IN ('applied', 'adopted')
      AND object_kind IN ('types', 'sequences', 'tables', 'synonyms', 'indexes',
                          'views', 'functions', 'procedures', 'triggers')
      AND live_definition_checksum IS NULL
)
    THROW 51000, 'cannot capture live managed-object state checksum', 1;

INSERT INTO azdo_deploy_meta.history (normalized_key, kind, checksum, live_definition_checksum, applied_ddl_version, git_hash, git_author, git_date, event, error_text, created_at)
SELECT
    normalized_key, kind, checksum, live_definition_checksum, applied_ddl_version, git_hash, git_author,
    git_date, event, error_text, SYSUTCDATETIME()
FROM @prepared;
