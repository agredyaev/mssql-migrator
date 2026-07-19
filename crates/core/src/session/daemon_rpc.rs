use crate::driver::{row::from_tiberius, RawClient, RowData};
use crate::error::Result;

use super::protocol::{Request, Response};

/// Dispatch one request against the (already exclusively held) session client.
pub async fn handle(client: &mut RawClient, req: Request) -> Response {
    match req {
        Request::Auth { .. } => Response::err("auth must be handled before rpc dispatch"),
        Request::Ping { .. } => match crate::driver::mssql::ping(client).await {
            Ok(()) => Response::ok_empty(),
            Err(e) => Response::err(e.to_string()),
        },
        Request::Exec { sql } => match crate::driver::mssql::exec(client, &sql).await {
            Ok(()) => Response::ok_empty(),
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

async fn query(client: &mut RawClient, sql: &str, params: &[String]) -> Result<Vec<RowData>> {
    let refs: Vec<&str> = params.iter().map(|s| s.as_str()).collect();
    let param_refs: Vec<&dyn tiberius::ToSql> =
        refs.iter().map(|s| s as &dyn tiberius::ToSql).collect();
    let rows = crate::driver::mssql::query_tiberius(client, sql, &param_refs).await?;
    Ok(rows.iter().map(from_tiberius).collect())
}

/// Roll back and release the session lock before reuse. The caller discards the
/// whole connection if either cleanup round-trip fails.
pub async fn cleanup_session(client: &mut RawClient) -> Result<()> {
    crate::driver::mssql::exec(client, crate::sql::apply::ROLLBACK_IF_OPEN).await?;
    crate::driver::mssql::exec(client, crate::sql::lock::RELEASE_IF_HELD).await
}
