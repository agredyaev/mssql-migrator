use crate::driver::{connect, mssql};
use crate::error::Result;

use super::Config;

pub async fn ensure_catalog_databases_exist(cfg: &Config, names: &[String]) -> Result<()> {
    if names.is_empty() {
        return Ok(());
    }
    let mut master_conn = None;
    for db in names {
        let mut target = cfg.clone();
        target.database = db.clone();
        let target_err = match connect(&target).await {
            Ok(_) => continue,
            Err(err) => err,
        };
        tracing::debug!(
            database = %db,
            sql_root = %cfg.sql_root,
            db_auth = %cfg.db_auth,
            error = %target_err,
            "target database probe failed"
        );

        if master_conn.is_none() {
            let mut master = cfg.clone();
            master.database = "master".into();
            let conn = match connect(&master).await {
                Ok(conn) => conn,
                Err(err) => {
                    tracing::warn!(
                        database = %db,
                        db_auth = %cfg.db_auth,
                        error = %err,
                        "master fallback connect failed while ensuring catalog database"
                    );
                    return Err(err);
                }
            };
            master_conn = Some(conn);
        }

        let Some(conn) = master_conn.as_mut() else {
            return Err(crate::error::Error::Sql(
                "master fallback connection missing while ensuring catalog database".into(),
            ));
        };
        let escaped = db.replace('\'', "''");
        let bracket = db.replace(']', "]]");
        let sql = format!("IF DB_ID(N'{escaped}') IS NULL CREATE DATABASE [{bracket}]");
        tracing::info!(
            database = %db,
            sql_root = %cfg.sql_root,
            "creating missing catalog database via master fallback"
        );
        mssql::exec(&mut conn.client, &sql).await?;
    }
    Ok(())
}
