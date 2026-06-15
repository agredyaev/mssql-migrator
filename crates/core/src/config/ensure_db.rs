use crate::driver::{connect, mssql, MssqlConn};
use crate::error::Result;

use super::Config;

pub async fn ensure_catalog_databases_exist(cfg: &Config, names: &[String]) -> Result<()> {
    if names.is_empty() {
        return Ok(());
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

async fn target_database_exists(cfg: &Config, db: &str) -> bool {
    let mut target = cfg.clone();
    target.database = db.to_owned();
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
    let escaped = db.replace('\'', "''");
    let bracket = db.replace(']', "]]");
    let sql = format!("IF DB_ID(N'{escaped}') IS NULL CREATE DATABASE [{bracket}]");
    tracing::info!(
        database = %db,
        sql_root = %cfg.sql_root,
        "creating missing catalog database via master fallback"
    );
    mssql::exec(&mut conn.client, &sql).await
}
