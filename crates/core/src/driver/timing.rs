use std::sync::{Arc, Mutex};
use std::time::Instant;

use super::io_profile::IoProfile;
use crate::driver::db_client::DbClient;
use crate::driver::row::RowData;
use crate::error::Result;

pub struct TimingConn {
    inner: Option<DbClient>,
    pub io: Arc<Mutex<IoProfile>>,
    _connect_done: Instant,
}

impl TimingConn {
    pub fn new(client: DbClient, io: Arc<Mutex<IoProfile>>, _connect_ms: i64) -> Self {
        Self {
            inner: Some(client),
            io,
            _connect_done: Instant::now(),
        }
    }

    pub fn client_mut(&mut self) -> &mut DbClient {
        self.inner.as_mut().expect("TimingConn client taken")
    }

    pub fn take_client(&mut self) -> DbClient {
        self.inner.take().expect("TimingConn client already taken")
    }

    pub fn restore_client(&mut self, client: DbClient) {
        assert!(self.inner.is_none(), "TimingConn client already present");
        self.inner = Some(client);
    }

    pub fn io_snapshot(&self) -> IoProfile {
        self.io.lock().unwrap().clone()
    }

    pub async fn exec(&mut self, sql: &str) -> Result<()> {
        let t0 = Instant::now();
        let r = self.client_mut().exec(sql).await;
        let ms = crate::timings::dur_ms(t0.elapsed());
        let mut io = self.io.lock().unwrap();
        io.exec_ms += ms;
        io.exec_calls += 1;
        r
    }

    pub async fn query(&mut self, sql: &str, params: &[&str]) -> Result<Vec<RowData>> {
        let t0 = Instant::now();
        let r = self.client_mut().query(sql, params).await;
        let ms = crate::timings::dur_ms(t0.elapsed());
        let mut io = self.io.lock().unwrap();
        io.query_ms += ms;
        io.query_calls += 1;
        r
    }

    pub async fn query_all(&mut self, sql: &str, params: &[&str]) -> Result<Vec<Vec<RowData>>> {
        let t0 = Instant::now();
        let r = self.client_mut().query_all(sql, params).await;
        let ms = crate::timings::dur_ms(t0.elapsed());
        let mut io = self.io.lock().unwrap();
        io.query_ms += ms;
        io.query_calls += 1;
        r
    }
}
