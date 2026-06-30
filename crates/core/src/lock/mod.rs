//! SQL Server session advisory locks and synchronization mutexes.
//!
//! ### Purpose
//! Prevents race conditions and dual-deployment conflicts by securing a unique distributed session lock
//! (`sp_getapplock`) in SQL Server before executing any schema mutations.
//!
//! ### Architectural Context
//! - **Inputs**: Active SQL Server connection, dynamic configuration options.
//! - **Outputs**: Mutex hold confirmation or execution block status.
//! - **Boundaries**: Scope is bound strictly to the active database connection session; closing the connection automatically releases the lock.
//!
//! ### Nominal Flow
//! 1. Open migration database session.
//! 2. Secure advisory lock using application mutex key (`acquire`).
//! 3. Execute planned schema alterations.
//! 4. Release the advisory lock after the guarded body returns (`release_after_body`), including on apply failure.
//!
//! ### Off-Nominal & Failure Containment
//! - **Lock Contention**: If another process currently holds the advisory lock, the execution blocks. If lock acquisition times out, returns `Error::LockTimeout` and exits with code `7` to fail safe.

use std::time::Duration;

use crate::config::Config;
use crate::driver::timing::TimingConn;
use crate::error::{Error, Result};
use crate::sql;

fn lock_timeout_millis_i32(timeout: Duration) -> i32 {
    timeout.as_millis().min(i32::MAX as u128) as i32
}

/// Release the session advisory lock and return the guarded body result.
pub async fn release_after_body<T>(conn: &mut TimingConn, body_result: Result<T>) -> Result<T> {
    if let Err(e) = release(conn).await {
        tracing::warn!(error = %e, "advisory lock release failed");
    }
    body_result
}

/// Acquires the SQL Server advisory lock, blocking for up to the configured timeout.
pub async fn acquire(conn: &mut TimingConn, cfg: &Config) -> Result<()> {
    let ms = lock_timeout_millis_i32(cfg.lock_timeout);
    let ms_str = ms.to_string();
    let rows = conn.query(sql::lock::ACQUIRE, &[ms_str.as_str()]).await?;
    let code = rows.first().and_then(|r| r.get_i32(0)).unwrap_or(-1);
    if code < 0 {
        return Err(Error::LockTimeout);
    }
    Ok(())
}

/// Releases the SQL Server advisory lock.
pub async fn release(conn: &mut TimingConn) -> Result<()> {
    conn.query(sql::lock::RELEASE, &[]).await?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use std::time::Duration;

    use super::lock_timeout_millis_i32;

    #[test]
    fn lock_timeout_millis_uses_configured_duration() {
        assert_eq!(lock_timeout_millis_i32(Duration::from_secs(3)), 3_000);
    }

    #[test]
    fn lock_timeout_millis_clamps_to_sql_server_int_limit() {
        assert_eq!(
            lock_timeout_millis_i32(Duration::from_millis(i32::MAX as u64 + 1)),
            i32::MAX
        );
    }
}
