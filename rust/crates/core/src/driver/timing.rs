use std::sync::{Arc, Mutex};
use std::time::Instant;

use super::io_profile::IoProfile;
use crate::driver::db_client::DbClient;
use crate::driver::row::RowData;
use crate::error::Result;

pub struct TimingConn {
    pub inner: DbClient,
    pub io: Arc<Mutex<IoProfile>>,
    _connect_done: Instant,
}

impl TimingConn {
    pub fn new(client: DbClient, io: Arc<Mutex<IoProfile>>, _connect_ms: i64) -> Self {
        Self {
            inner: client,
            io,
            _connect_done: Instant::now(),
        }
    }

    pub fn io_snapshot(&self) -> IoProfile {
        self.io.lock().unwrap().clone()
    }

    pub async fn exec(&mut self, sql: &str) -> Result<()> {
        let t0 = Instant::now();
        let r = self.inner.exec(sql).await;
        let ms = crate::timings::dur_ms(t0.elapsed());
        let mut io = self.io.lock().unwrap();
        io.exec_ms += ms;
        io.exec_calls += 1;
        r
    }

    pub async fn query(&mut self, sql: &str, params: &[&str]) -> Result<Vec<RowData>> {
        let t0 = Instant::now();
        let r = self.inner.query(sql, params).await;
        let ms = crate::timings::dur_ms(t0.elapsed());
        let mut io = self.io.lock().unwrap();
        io.query_ms += ms;
        io.query_calls += 1;
        r
    }

    pub async fn query_all(&mut self, sql: &str, params: &[&str]) -> Result<Vec<Vec<RowData>>> {
        let t0 = Instant::now();
        let r = self.inner.query_all(sql, params).await;
        let ms = crate::timings::dur_ms(t0.elapsed());
        let mut io = self.io.lock().unwrap();
        io.query_ms += ms;
        io.query_calls += 1;
        r
    }
}
