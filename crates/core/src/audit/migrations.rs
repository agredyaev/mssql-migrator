use std::collections::HashMap;

use crate::driver::DbClient;
use crate::error::Result;
use crate::sql;

pub async fn load_all_applied(client: &mut DbClient) -> Result<HashMap<String, bool>> {
    let rows = client.query(sql::audit::LOAD_ALL_MIGRATIONS, &[]).await?;
    let mut out = HashMap::new();
    for row in rows {
        let key = row.get_str(0).unwrap_or("");
        if !key.is_empty() {
            out.insert(key.to_string(), true);
        }
    }
    Ok(out)
}
