//! [`TimingConn`] — [`DbClient`] wrapper that records per-query I/O timing.

use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};

use super::io_profile::{lock_unpoisoned, IoProfile};
use crate::driver::db_client::DbClient;
use crate::driver::row::RowData;
use crate::error::{Error, Result};

/// Database connection wrapper that records per-call timing into an `IoProfile`.
pub struct TimingConn {
    inner: Option<DbClient>,
    /// Shared I/O profile accumulator; updated after every exec/query call.
    pub io: Arc<Mutex<IoProfile>>,
    /// Per-command execution timeout. `Duration::ZERO` disables it (the default,
    /// so test connections constructed via `new` run unbounded).
    command_timeout: Duration,
}

impl TimingConn {
    /// Creates a `TimingConn` wrapping `client` and sharing the given `io` profile.
    pub fn new(client: DbClient, io: Arc<Mutex<IoProfile>>) -> Self {
        Self {
            inner: Some(client),
            io,
            command_timeout: Duration::ZERO,
        }
    }

    /// Bound every `exec`/`query` call to `timeout`. `Duration::ZERO` disables it.
    /// Without this a hung SQL Server would block the CLI forever in CI.
    pub fn set_command_timeout(&mut self, timeout: Duration) {
        self.command_timeout = timeout;
    }

    /// Returns a mutable reference to the inner `DbClient`.
    pub fn client_mut(&mut self) -> Result<&mut DbClient> {
        self.inner
            .as_mut()
            .ok_or_else(|| Error::Sql("TimingConn client is temporarily unavailable".into()))
    }

    /// Returns a clone of the current `IoProfile` accumulated by this connection.
    pub fn io_snapshot(&self) -> IoProfile {
        lock_unpoisoned(&self.io).clone()
    }

    /// A timed-out call left the TDS stream mid-protocol; drop the client so any
    /// later use fails fast instead of reading a stale response.
    fn timeout_err(&mut self, op: &str, timeout: Duration) -> Error {
        self.inner = None;
        Error::Sql(format!("{op} timed out after {timeout:?}"))
    }

    /// Executes a non-returning SQL statement and records execution time.
    pub async fn exec(&mut self, sql: &str) -> Result<()> {
        let t0 = Instant::now();
        let timeout = self.command_timeout;
        let r = match run_bounded(timeout, self.client_mut()?.exec(sql)).await {
            Some(r) => r,
            None => Err(self.timeout_err("exec", timeout)),
        };
        let ms = crate::timings::dur_ms(t0.elapsed());
        let mut io = lock_unpoisoned(&self.io);
        io.exec_ms += ms;
        io.exec_calls += 1;
        r
    }

    /// Executes a parameterised SQL query and returns the result rows.
    pub async fn query(&mut self, sql: &str, params: &[&str]) -> Result<Vec<RowData>> {
        let t0 = Instant::now();
        let timeout = self.command_timeout;
        let r = match run_bounded(timeout, self.client_mut()?.query(sql, params)).await {
            Some(r) => r,
            None => Err(self.timeout_err("query", timeout)),
        };
        let ms = crate::timings::dur_ms(t0.elapsed());
        let mut io = lock_unpoisoned(&self.io);
        io.query_ms += ms;
        io.query_calls += 1;
        r
    }

    /// Executes a parameterised SQL query and returns all result sets.
    pub async fn query_all(&mut self, sql: &str, params: &[&str]) -> Result<Vec<Vec<RowData>>> {
        let t0 = Instant::now();
        let timeout = self.command_timeout;
        let r = match run_bounded(timeout, self.client_mut()?.query_all(sql, params)).await {
            Some(r) => r,
            None => Err(self.timeout_err("query", timeout)),
        };
        let ms = crate::timings::dur_ms(t0.elapsed());
        let mut io = lock_unpoisoned(&self.io);
        io.query_ms += ms;
        io.query_calls += 1;
        r
    }
}

/// Runs `fut` under `timeout`. `Duration::ZERO` runs it unbounded; otherwise a
/// `tokio` timeout wraps it, mapping an elapsed timeout to `None`.
async fn run_bounded<Fut, T>(timeout: Duration, fut: Fut) -> Option<Result<T>>
where
    Fut: std::future::Future<Output = Result<T>>,
{
    if timeout.is_zero() {
        Some(fut.await)
    } else {
        tokio::time::timeout(timeout, fut).await.ok()
    }
}
