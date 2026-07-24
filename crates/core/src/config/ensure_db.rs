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
        if target_database_exists(cfg, db).await? {
            continue;
        }
        ensure_database_from_master(cfg, db, &mut master_conn).await?;
    }
    Ok(())
}

/// Probe whether catalog database `db` exists on the server by attempting a
/// direct connect. `Ok(false)` means the database is absent (SQL 4060 "cannot
/// open database"); any other failure (TCP refused, timeout, auth) is an
/// infrastructure problem and propagates as `Err` rather than being silently
/// reported as "not present" (which would let an outage pass as exit 0).
pub async fn target_database_exists(cfg: &Config, db: &str) -> Result<bool> {
    let mut target = cfg.clone();
    target.database = db.into();
    match connect(&target).await {
        Ok(_) => Ok(true),
        Err(err) => {
            if is_missing_db_error(&err.to_string()) {
                tracing::debug!(database = %db, "target database absent");
                Ok(false)
            } else {
                tracing::warn!(database = %db, error = %err, "target database probe failed (infrastructure)");
                Err(err)
            }
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
    let sql = format!(
        include_str!("../../../../sql/apply/create_database.sql"),
        literal = literal,
        bracket = bracket,
    );
    tracing::info!(
        database = %db,
        sql_root = %cfg.sql_root,
        "creating missing catalog database via master fallback"
    );
    // First-deployment recovery must stay inside RM_COMMAND_TIMEOUT: this exec
    // runs on a raw master connection with no TimingConn bound around it.
    let t = cfg.command_timeout;
    with_create_database_timeout(t, db, mssql::exec(&mut conn.client, &sql)).await
}

async fn with_create_database_timeout<F, T>(
    timeout: std::time::Duration,
    db: &str,
    fut: F,
) -> Result<T>
where
    F: std::future::Future<Output = Result<T>>,
{
    if timeout.is_zero() {
        fut.await
    } else {
        tokio::time::timeout(timeout, fut).await.map_err(|_| {
            crate::error::Error::Sql(format!("CREATE DATABASE {db} timed out after {timeout:?}"))
            // sql-gate:allow error text
        })?
    }
}

// Match only SQL Server's own 4060 message text. A bare "4060" substring also
// matches hostnames/ports in transport errors and would classify an outage as
// "database absent".
fn is_missing_db_error(msg: &str) -> bool {
    msg.to_lowercase().contains("cannot open database")
}

#[cfg(test)]
#[path = "../tests/ensure_db_test.rs"]
mod ensure_db_tests;
