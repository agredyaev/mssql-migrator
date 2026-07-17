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

/// Full session cleanup when a client disconnects on ANY path: roll back any
/// transaction the dying client left open, then release its advisory lock —
/// the shared connection must hand the next socket a neutral session.
pub async fn cleanup_session(client: &mut RawClient) {
    const ROLLBACK_IF_OPEN: &str = "IF @@TRANCOUNT > 0 ROLLBACK TRANSACTION;";
    let _ = crate::driver::mssql::exec(client, ROLLBACK_IF_OPEN).await;
    release_session_lock(client).await;
}

/// Best-effort release of the session advisory lock when a client disconnects,
/// so a client that died mid-deploy without releasing does not leave the shared
/// daemon session holding `reporting_layer_migration` forever. Releases ONLY if
/// the session currently holds it, so a read-only client that never acquired the
/// lock does not log a spurious "not currently held" error.
pub async fn release_session_lock(client: &mut RawClient) {
    const RELEASE_IF_HELD: &str = "IF APPLOCK_MODE('public', 'reporting_layer_migration', 'Session') <> 'NoLock' EXEC sp_releaseapplock @Resource = 'reporting_layer_migration', @LockOwner = 'Session';";
    let _ = crate::driver::mssql::exec(client, RELEASE_IF_HELD).await;
}
