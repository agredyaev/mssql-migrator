//! Shared TDS connection wrapper for parallel plan-DB phases.
//!
//! ### Purpose
//! [`SharedConn`] wraps a `DbClient` behind `Arc<tokio::sync::Mutex>` so that
//! concurrent plan-DB tasks (body + ensure) share a single TDS connection
//! without extra login overhead. I/O profiling counters are behind a separate
//! `Arc<Mutex<IoProfile>>` and are aggregated across tasks.
//!
//! ### Non-obvious
//! - The `DbClient` mutex is a *tokio* async mutex (not `std::sync::Mutex`)
//!   because the lock is held across `.await` points during query execution.
//! - The `IoProfile` mutex is a *std* mutex (brief lock, no `.await` held).
//! - I/O profile mutex poisoning is recovered with the existing counters. A
//!   panic in metrics bookkeeping must not turn into a later plan panic.

use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};

use crate::driver::{io_profile::lock_profile, DbClient, IoProfile, RowData, TimingConn};
use crate::error::{Error, Result};
use crate::timings;

pub(crate) struct SharedConn {
    /// Shared TDS connection (tokio mutex — held across `.await`).
    pub client: Arc<tokio::sync::Mutex<DbClient>>,
    /// Shared IO profile counters (std mutex — no `.await` held).
    pub io: Arc<Mutex<IoProfile>>,
    /// Per-call timeout; `Duration::ZERO` disables it.
    pub timeout: Duration,
}

/// Bound `fut` by `t` after the connection lock is already held, so lock-wait
/// time does not count against the command timeout.
pub(crate) async fn bounded<T>(
    t: Duration,
    what: &str,
    fut: impl std::future::Future<Output = Result<T>>,
) -> Result<T> {
    if t.is_zero() {
        return fut.await;
    }
    match tokio::time::timeout(t, fut).await {
        Ok(r) => r,
        Err(_) => Err(Error::Sql(format!("{what} timed out after {t:?}"))),
    }
}

impl SharedConn {
    /// Execute a SQL statement, recording timing in the shared IO profile.
    pub async fn exec(&self, sql: &str) -> Result<()> {
        let t0 = Instant::now();
        let mut c = self.client.lock().await;
        let r = bounded(self.timeout, "exec", c.exec(sql)).await;
        let ms = timings::dur_ms(t0.elapsed());
        let mut io = lock_profile(&self.io);
        io.exec_ms += ms;
        io.exec_calls += 1;
        r
    }

    /// Run a SQL query with params, recording timing in the shared IO profile.
    pub async fn query(&self, sql: &str, params: &[&str]) -> Result<Vec<RowData>> {
        let t0 = Instant::now();
        let mut c = self.client.lock().await;
        let r = bounded(self.timeout, "query", c.query(sql, params)).await;
        let ms = timings::dur_ms(t0.elapsed());
        let mut io = lock_profile(&self.io);
        io.query_ms += ms;
        io.query_calls += 1;
        r
    }

    /// Run a batched SQL query with params, recording timing.
    pub async fn query_all(&self, sql: &str, params: &[&str]) -> Result<Vec<Vec<RowData>>> {
        let t0 = Instant::now();
        let mut c = self.client.lock().await;
        let r = bounded(self.timeout, "query", c.query_all(sql, params)).await;
        let ms = timings::dur_ms(t0.elapsed());
        let mut io = lock_profile(&self.io);
        io.query_ms += ms;
        io.query_calls += 1;
        r
    }
}

pub(crate) enum PlanDbConn<'a> {
    Timing(&'a mut TimingConn),
    Shared(&'a SharedConn),
}

impl PlanDbConn<'_> {
    pub async fn exec(&mut self, sql: &str) -> Result<()> {
        match self {
            Self::Timing(c) => c.exec(sql).await,
            Self::Shared(c) => c.exec(sql).await,
        }
    }

    pub async fn query(&mut self, sql: &str, params: &[&str]) -> Result<Vec<RowData>> {
        match self {
            Self::Timing(c) => c.query(sql, params).await,
            Self::Shared(c) => c.query(sql, params).await,
        }
    }

    pub async fn query_all(&mut self, sql: &str, params: &[&str]) -> Result<Vec<Vec<RowData>>> {
        match self {
            Self::Timing(c) => c.query_all(sql, params).await,
            Self::Shared(c) => c.query_all(sql, params).await,
        }
    }
}
