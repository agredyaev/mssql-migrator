//! Workflow integration DB reset (full DROP/CREATE or fast smoke wipe).

use migrator_core::config::Config;
use migrator_core::driver::{connect, mssql};
use migrator_core::error::Result;

use super::db_reset::{invalidate_process_caches, reset_test_database};
#[path = "db_reset_skip.rs"]
mod db_reset_skip;

const FAST_RESET_SQL: &str = r#"
DECLARE @sql NVARCHAR(MAX) = N'';
SELECT @sql = @sql
    + N'DROP '
    + CASE o.type
        WHEN 'V' THEN 'VIEW '
        WHEN 'P' THEN 'PROCEDURE '
        WHEN 'FN' THEN 'FUNCTION '
        WHEN 'IF' THEN 'FUNCTION '
        WHEN 'TF' THEN 'FUNCTION '
        WHEN 'U' THEN 'TABLE '
        ELSE NULL
      END
    + QUOTENAME(OBJECT_SCHEMA_NAME(o.object_id)) + N'.' + QUOTENAME(o.name) + N';' + CHAR(10)
FROM sys.objects o
WHERE OBJECT_SCHEMA_NAME(o.object_id) = N'smoke'
  AND o.type IN ('V', 'P', 'FN', 'IF', 'TF', 'U')
  AND o.is_ms_shipped = 0;
IF LEN(@sql) > 0
    EXEC sp_executesql @sql;
IF SCHEMA_ID(N'smoke') IS NOT NULL
    DROP SCHEMA smoke;
IF OBJECT_ID(N'azdo_deploy_meta.catalog_cache') IS NOT NULL
    DELETE FROM azdo_deploy_meta.catalog_cache;
IF OBJECT_ID(N'azdo_deploy_meta.catalog_meta') IS NOT NULL
    DELETE FROM azdo_deploy_meta.catalog_meta;
IF OBJECT_ID(N'azdo_deploy_meta.history') IS NOT NULL
    DELETE FROM azdo_deploy_meta.history;
"#;

fn workflow_fast_reset() -> bool {
    matches!(
        std::env::var("RMIG_WORKFLOW_FAST_RESET").as_deref(),
        Ok("1") | Ok("true") | Ok("yes")
    )
}

async fn fast_reset_test_database(cfg: &Config) -> Result<()> {
    let mut conn = connect(cfg).await?;
    mssql::exec(&mut conn.client, FAST_RESET_SQL).await?;
    invalidate_process_caches(cfg, false).await?;
    let db_fp =
        migrator_core::audit::db_fingerprint(&cfg.server, &cfg.port, &cfg.user, &cfg.database);
    migrator_core::audit::mark_tables_ensured(&db_fp);
    Ok(())
}

/// Entry point for workflow integration: full, fast, or skip reset.
pub async fn prepare_test_database(cfg: &Config) -> Result<()> {
    if db_reset_skip::skip_db_reset() {
        return Ok(());
    }
    if workflow_fast_reset() {
        fast_reset_test_database(cfg).await
    } else {
        reset_test_database(cfg).await
    }
}
