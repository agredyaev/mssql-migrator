use crate::driver::mssql::RawClient;
use crate::error::{Error, Result};

pub async fn ping(client: &mut RawClient) -> Result<()> {
    client
        .simple_query("SELECT 1")
        .await
        .map_err(|e| Error::Sql(e.to_string()))?;
    Ok(())
}

pub async fn exec(client: &mut RawClient, sql: &str) -> Result<()> {
    client
        .simple_query(sql)
        .await
        .map_err(|e| Error::Sql(e.to_string()))?
        .into_results()
        .await
        .map_err(|e| Error::Sql(e.to_string()))?;
    Ok(())
}

pub async fn query_tiberius(
    client: &mut RawClient,
    sql: &str,
    params: &[&dyn tiberius::ToSql],
) -> Result<Vec<tiberius::Row>> {
    client
        .query(sql, params)
        .await
        .map_err(|e| Error::Sql(e.to_string()))?
        .into_first_result()
        .await
        .map_err(|e| Error::Sql(e.to_string()))
}

/// All result sets from one batch (single round-trip).
pub async fn query_all_results(
    client: &mut RawClient,
    sql: &str,
    params: &[&dyn tiberius::ToSql],
) -> Result<Vec<Vec<tiberius::Row>>> {
    client
        .query(sql, params)
        .await
        .map_err(|e| Error::Sql(e.to_string()))?
        .into_results()
        .await
        .map_err(|e| Error::Sql(e.to_string()))
}
