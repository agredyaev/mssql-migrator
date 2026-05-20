use crate::error::Result;
use crate::session::ProxyClient;

use super::mssql::{self, RawClient};
use super::row::RowData;

pub enum DbClient {
    Direct(RawClient),
    Proxy(ProxyClient),
}

impl DbClient {
    pub async fn exec(&mut self, sql: &str) -> Result<()> {
        match self {
            Self::Direct(c) => mssql::exec(c, sql).await,
            Self::Proxy(p) => p.exec(sql).await,
        }
    }

    pub async fn query(&mut self, sql: &str, params: &[&str]) -> Result<Vec<RowData>> {
        match self {
            Self::Direct(c) => {
                let refs: Vec<&dyn tiberius::ToSql> =
                    params.iter().map(|s| s as &dyn tiberius::ToSql).collect();
                let rows = mssql::query_tiberius(c, sql, &refs).await?;
                Ok(rows.iter().map(super::row::from_tiberius).collect())
            }
            Self::Proxy(p) => p.query(sql, params).await,
        }
    }

    pub async fn query_all(&mut self, sql: &str, params: &[&str]) -> Result<Vec<Vec<RowData>>> {
        match self {
            Self::Direct(c) => {
                let refs: Vec<&dyn tiberius::ToSql> =
                    params.iter().map(|s| s as &dyn tiberius::ToSql).collect();
                let sets = mssql::query_all_results(c, sql, &refs).await?;
                Ok(sets
                    .into_iter()
                    .map(|set| set.iter().map(super::row::from_tiberius).collect())
                    .collect())
            }
            Self::Proxy(p) => {
                let rows = p.query(sql, params).await?;
                Ok(vec![rows])
            }
        }
    }
}
