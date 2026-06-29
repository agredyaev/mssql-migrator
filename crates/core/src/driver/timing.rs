use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};

use super::io_profile::{lock_profile, IoProfile};
use crate::driver::db_client::DbClient;
use crate::driver::row::RowData;
use crate::error::{Error, Result};

pub struct TimingConn {
    inner: Option<DbClient>,
    pub io: Arc<Mutex<IoProfile>>,
    _connect_done: Instant,
    /// Per-command execution timeout. `Duration::ZERO` disables it (the default,
    /// so test connections constructed via `new` run unbounded).
    command_timeout: Duration,
}

impl TimingConn {
    pub fn new(client: DbClient, io: Arc<Mutex<IoProfile>>, _connect_ms: i64) -> Self {
        Self {
            inner: Some(client),
            io,
            _connect_done: Instant::now(),
            command_timeout: Duration::ZERO,
        }
    }

    /// Bound every `exec`/`query` call to `timeout`. `Duration::ZERO` disables it.
    /// Without this a hung SQL Server would block the CLI forever in CI.
    pub fn set_command_timeout(&mut self, timeout: Duration) {
        self.command_timeout = timeout;
    }

    pub fn client_mut(&mut self) -> Result<&mut DbClient> {
        self.inner
            .as_mut()
            .ok_or_else(|| Error::Sql("TimingConn client is temporarily unavailable".into()))
    }

    pub fn take_client(&mut self) -> Result<DbClient> {
        self.inner
            .take()
            .ok_or_else(|| Error::Sql("TimingConn client was already taken".into()))
    }

    pub fn restore_client(&mut self, client: DbClient) -> Result<()> {
        if self.inner.is_some() {
            return Err(Error::Sql("TimingConn client already present".into()));
        }
        self.inner = Some(client);
        Ok(())
    }

    pub fn io_snapshot(&self) -> IoProfile {
        lock_profile(&self.io).clone()
    }

    pub async fn exec(&mut self, sql: &str) -> Result<()> {
        let t0 = Instant::now();
        let timeout = self.command_timeout;
        let r = if timeout.is_zero() {
            self.client_mut()?.exec(sql).await
        } else {
            match tokio::time::timeout(timeout, self.client_mut()?.exec(sql)).await {
                Ok(r) => r,
                Err(_) => Err(Error::Sql(format!("exec timed out after {timeout:?}"))),
            }
        };
        let ms = crate::timings::dur_ms(t0.elapsed());
        let mut io = lock_profile(&self.io);
        io.exec_ms += ms;
        io.exec_calls += 1;
        r
    }

    pub async fn query(&mut self, sql: &str, params: &[&str]) -> Result<Vec<RowData>> {
        let t0 = Instant::now();
        let timeout = self.command_timeout;
        let r = if timeout.is_zero() {
            self.client_mut()?.query(sql, params).await
        } else {
            match tokio::time::timeout(timeout, self.client_mut()?.query(sql, params)).await {
                Ok(r) => r,
                Err(_) => Err(Error::Sql(format!("query timed out after {timeout:?}"))),
            }
        };
        let ms = crate::timings::dur_ms(t0.elapsed());
        let mut io = lock_profile(&self.io);
        io.query_ms += ms;
        io.query_calls += 1;
        r
    }

    pub async fn query_all(&mut self, sql: &str, params: &[&str]) -> Result<Vec<Vec<RowData>>> {
        let t0 = Instant::now();
        let timeout = self.command_timeout;
        let r = if timeout.is_zero() {
            self.client_mut()?.query_all(sql, params).await
        } else {
            match tokio::time::timeout(timeout, self.client_mut()?.query_all(sql, params)).await {
                Ok(r) => r,
                Err(_) => Err(Error::Sql(format!("query timed out after {timeout:?}"))),
            }
        };
        let ms = crate::timings::dur_ms(t0.elapsed());
        let mut io = lock_profile(&self.io);
        io.query_ms += ms;
        io.query_calls += 1;
        r
    }
}
