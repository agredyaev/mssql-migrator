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
//! 4. Release the advisory lock dynamically on completion (`release`).
//!
//! ### Off-Nominal & Failure Containment
//! - **Lock Contention**: If another process currently holds the advisory lock, the execution blocks. If lock acquisition times out, returns `Error::LockTimeout` and exits with code `7` to fail safe.

use crate::config::Config;
use crate::driver::timing::TimingConn;
use crate::error::{Error, Result};
use crate::sql;

pub async fn acquire(conn: &mut TimingConn, cfg: &Config) -> Result<()> {
    let ms = cfg.lock_timeout.as_millis() as i32;
    let ms_str = ms.to_string();
    let rows = conn.query(sql::lock::ACQUIRE, &[ms_str.as_str()]).await?;
    let code = rows.first().and_then(|r| r.get_i32(0)).unwrap_or(-1);
    if code < 0 {
        return Err(Error::LockTimeout);
    }
    Ok(())
}

pub async fn release(conn: &mut TimingConn) -> Result<()> {
    let _ = conn.query(sql::lock::RELEASE, &[]).await?;
    Ok(())
}
