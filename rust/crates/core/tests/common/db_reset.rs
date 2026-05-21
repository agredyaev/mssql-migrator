//! DROP/CREATE test database (shared by Go↔Rust e2e and workflow).

use migrator_core::audit::{
    self, invalidate_audit_cache, invalidate_audit_cache_all, mark_tables_ensured,
};
use migrator_core::cache::l1::L1Cache;
use migrator_core::config::Config;
use migrator_core::driver::{connect, mssql};
use migrator_core::error::Result;
use migrator_core::sql;

pub async fn reset_test_database(cfg: &Config) -> Result<()> {
    let mut master = cfg.clone();
    master.database = "master".into();
    let mut conn = connect(&master).await?;
    let sql = format!(
        r#"
IF DB_ID('{db}') IS NOT NULL
BEGIN
    ALTER DATABASE [{db}] SET SINGLE_USER WITH ROLLBACK IMMEDIATE;
    DROP DATABASE [{db}];
END
CREATE DATABASE [{db}]
"#,
        db = cfg.database.replace('\'', "''")
    );
    mssql::exec(&mut conn.client, &sql).await?;
    invalidate_process_caches(cfg, true).await?;
    prebootstrap_audit_tables(cfg).await?;
    Ok(())
}

pub(crate) async fn invalidate_process_caches(cfg: &Config, full: bool) -> Result<()> {
    let db_fp = audit::db_fingerprint(&cfg.server, &cfg.database);
    if full {
        invalidate_audit_cache_all(&db_fp);
    } else {
        invalidate_audit_cache(&db_fp);
    }
    let l1 = L1Cache::new(&cfg.l1_cache_dir);
    let _ = l1.invalidate_all(&db_fp);
    Ok(())
}

pub(crate) async fn prebootstrap_audit_tables(cfg: &Config) -> Result<()> {
    let mut conn = connect(cfg).await?;
    let bootstrap = format!(
        "{}\n{}",
        sql::audit::BOOTSTRAP_TABLES,
        sql::audit::BOOTSTRAP_INDEX
    );
    mssql::exec(&mut conn.client, &bootstrap).await?;
    let db_fp = audit::db_fingerprint(&cfg.server, &cfg.database);
    mark_tables_ensured(&db_fp);
    Ok(())
}
