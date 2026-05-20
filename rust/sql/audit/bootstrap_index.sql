IF OBJECT_ID('azdo_deploy_meta.history') IS NOT NULL
   AND NOT EXISTS (
        SELECT 1
        FROM sys.indexes
        WHERE name = 'IX_history_key_kind'
          AND object_id = OBJECT_ID('azdo_deploy_meta.history')
    )
    CREATE INDEX IX_history_key_kind ON azdo_deploy_meta.history(normalized_key, kind, id DESC);
