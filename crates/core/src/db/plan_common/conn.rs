use std::sync::{Arc, Mutex};
use std::time::Instant;

use crate::driver::{DbClient, IoProfile, RowData, TimingConn};
use crate::error::Result;
use crate::timings;

pub(crate) struct SharedConn {
    pub client: Arc<tokio::sync::Mutex<DbClient>>,
    pub io: Arc<Mutex<IoProfile>>,
}

impl SharedConn {
    pub async fn exec(&self, sql: &str) -> Result<()> {
        let t0 = Instant::now();
        let r = self.client.lock().await.exec(sql).await;
        let ms = timings::dur_ms(t0.elapsed());
        let mut io = self.io.lock().unwrap();
        io.exec_ms += ms;
        io.exec_calls += 1;
        r
    }

    pub async fn query(&self, sql: &str, params: &[&str]) -> Result<Vec<RowData>> {
        let t0 = Instant::now();
        let r = self.client.lock().await.query(sql, params).await;
        let ms = timings::dur_ms(t0.elapsed());
        let mut io = self.io.lock().unwrap();
        io.query_ms += ms;
        io.query_calls += 1;
        r
    }

    pub async fn query_all(&self, sql: &str, params: &[&str]) -> Result<Vec<Vec<RowData>>> {
        let t0 = Instant::now();
        let r = self.client.lock().await.query_all(sql, params).await;
        let ms = timings::dur_ms(t0.elapsed());
        let mut io = self.io.lock().unwrap();
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
