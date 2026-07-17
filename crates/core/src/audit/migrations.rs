use std::collections::HashMap;

use crate::driver::DbClient;
use crate::error::Result;
use crate::sql;

/// Loads all applied transition keys from the audit history, mapped to the
/// latest recorded checksum (hex; empty when the stored value is unusable).
/// The checksum lets the filter detect an applied script whose file was edited
/// afterwards instead of silently dropping it.
pub async fn load_all_applied(client: &mut DbClient) -> Result<HashMap<String, String>> {
    let rows = client.query(sql::audit::LOAD_ALL_MIGRATIONS, &[]).await?;
    let mut out = HashMap::new();
    for row in rows {
        let key = row.get_str(0).unwrap_or("");
        if !key.is_empty() {
            let cs = row.get_str(1).unwrap_or("").trim().to_string();
            out.insert(key.to_string(), cs);
        }
    }
    Ok(out)
}
