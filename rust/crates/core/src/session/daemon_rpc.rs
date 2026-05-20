use std::sync::Arc;

use tokio::sync::Mutex;

use crate::driver::{row::from_tiberius, RawClient, RowData};
use crate::error::Result;

use super::protocol::{Request, Response};

pub async fn handle(
    client: &Arc<Mutex<RawClient>>,
    req: Request,
) -> Response {
    match req {
        Request::Auth { .. } => Response::err("auth must be handled before rpc dispatch"),
        Request::Ping => match ping(client).await {
            Ok(()) => Response {
                ok: true,
                error: String::new(),
                rows: Vec::new(),
            },
            Err(e) => Response::err(e.to_string()),
        },
        Request::Exec { sql } => match exec(client, &sql).await {
            Ok(()) => Response {
                ok: true,
                error: String::new(),
                rows: Vec::new(),
            },
            Err(e) => Response::err(e.to_string()),
        },
        Request::Query { sql, params } => match query(client, &sql, &params).await {
            Ok(rows) => Response {
                ok: true,
                error: String::new(),
                rows,
            },
            Err(e) => Response::err(e.to_string()),
        },
    }
}

async fn ping(client: &Arc<Mutex<RawClient>>) -> Result<()> {
    let mut c = client.lock().await;
    crate::driver::mssql::ping(&mut c).await
}

async fn exec(client: &Arc<Mutex<RawClient>>, sql: &str) -> Result<()> {
    let mut c = client.lock().await;
    crate::driver::mssql::exec(&mut c, sql).await
}

async fn query(
    client: &Arc<Mutex<RawClient>>,
    sql: &str,
    params: &[String],
) -> Result<Vec<RowData>> {
    let mut c = client.lock().await;
    let refs: Vec<&str> = params.iter().map(|s| s.as_str()).collect();
    let param_refs: Vec<&dyn tiberius::ToSql> = refs
        .iter()
        .map(|s| s as &dyn tiberius::ToSql)
        .collect();
    let rows = crate::driver::mssql::query_tiberius(&mut c, sql, &param_refs).await?;
    Ok(rows.iter().map(from_tiberius).collect())
}
