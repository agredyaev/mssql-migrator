use crate::driver::{connect, mssql, MssqlConn};
use crate::error::Result;
use crate::sql_ident::bracket_ident;

use super::Config;

/// Ensures all catalog database names in `names` exist on the server, creating them if absent.
pub async fn ensure_catalog_databases_exist(cfg: &Config, names: &[String]) -> Result<()> {
    if names.is_empty() {
        return Ok(());
    }
    // Validate every catalog name before opening any connection so an illegal or
    // over-long identifier fails fast with InvalidInput (exit 8) instead of an
    // opaque SQL error mid-deploy.
    for db in names {
        bracket_ident(db)?;
    }
    let mut master_conn = None;
    for db in names {
        if target_database_exists(cfg, db).await {
            continue;
        }
        ensure_database_from_master(cfg, db, &mut master_conn).await?;
    }
    Ok(())
}

/// Probe whether catalog database `db` exists on the server by attempting a
/// direct connect. A failed connect (e.g. SQL 4060 "cannot open database") is
/// treated as "not present" rather than a hard error, so callers can decide
/// whether to create it (mutating commands) or skip it (read-only multi-DB plan).
pub async fn target_database_exists(cfg: &Config, db: &str) -> bool {
    let mut target = cfg.clone();
    target.database = db.into();
    match connect(&target).await {
        Ok(_) => true,
        Err(err) => {
            tracing::debug!(
                database = %db,
                sql_root = %cfg.sql_root,
                db_auth = %cfg.db_auth,
                error = %err,
                "target database probe failed"
            );
            false
        }
    }
}

async fn ensure_database_from_master(
    cfg: &Config,
    db: &str,
    master_conn: &mut Option<MssqlConn>,
) -> Result<()> {
    if master_conn.is_none() {
        *master_conn = Some(connect_master(cfg, db).await?);
    }

    let Some(conn) = master_conn.as_mut() else {
        return Err(crate::error::Error::Sql(
            "master fallback connection missing while ensuring catalog database".into(),
        ));
    };
    create_database_if_missing(conn, cfg, db).await
}

async fn connect_master(cfg: &Config, db: &str) -> Result<MssqlConn> {
    let mut master = cfg.clone();
    master.database = "master".into();
    connect(&master).await.map_err(|err| {
        tracing::warn!(
            database = %db,
            db_auth = %cfg.db_auth,
            error = %err,
            "master fallback connect failed while ensuring catalog database"
        );
        err
    })
}

async fn create_database_if_missing(conn: &mut MssqlConn, cfg: &Config, db: &str) -> Result<()> {
    // `bracket_ident` validates + wraps as `[db]` (escaping `]`); the `''` escape
    // is still needed for the `N'...'` string-literal context of the existence probe.
    let bracket = bracket_ident(db)?;
    let literal = db.replace('\'', "''");
    let sql = format!("IF DB_ID(N'{literal}') IS NULL CREATE DATABASE {bracket}");
    tracing::info!(
        database = %db,
        sql_root = %cfg.sql_root,
        "creating missing catalog database via master fallback"
    );
    mssql::exec(&mut conn.client, &sql).await
}

#[cfg(test)]
#[path = "../tests/ensure_db_test.rs"]
mod ensure_db_tests;
