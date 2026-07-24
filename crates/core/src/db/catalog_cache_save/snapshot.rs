use super::save::save_batched;
use crate::db::state::{catalog_object_parts, CatalogState};
use crate::domain::Workspace;
use crate::driver::TimingConn;
use crate::error::Result;
use crate::sql;

/// Persist workspace object metadata after a successful apply (warm catalog_cache for git delta).
pub async fn save_workspace_snapshot(
    conn: &mut TimingConn,
    layout_digest: &[u8; 32],
    ws: &Workspace,
    enabled: bool,
) -> Result<()> {
    if !enabled || ws.object_count() == 0 {
        return Ok(());
    }
    let mut state = CatalogState::default();
    for i in 0..ws.object_count() {
        let obj = ws.entry(i);
        state.schemas.insert(obj.key.schema_part().to_lowercase());
        state.objects.insert(
            ws.entry_key(i).clone(),
            catalog_object_parts(
                obj.key.schema_shared(),
                obj.key.kind_shared(),
                obj.key.name_shared(),
                obj.parent
                    .filter(|p| p.parent_row_id > 0)
                    .map(|_| obj.parent_name(ws)),
            ),
        );
    }
    hydrate_index_parents_from_db(conn, &mut state).await?;
    save_batched(conn, layout_digest, ws, &state).await
}

/// After cold apply, workspace parent refs for indexes are empty; read live sys.indexes parents.
async fn hydrate_index_parents_from_db(
    conn: &mut TimingConn,
    state: &mut CatalogState,
) -> Result<()> {
    let needs = state
        .objects
        .values()
        .any(|o| o.kind == "indexes" && o.parent.as_ref().is_none_or(String::is_empty));
    if !needs {
        return Ok(());
    }
    let rows = conn.query(sql::catalog::INDEX_PARENTS, &[]).await?;
    for row in rows {
        let schema = row.get_str(0).unwrap_or("").to_lowercase();
        let index = row.get_str(1).unwrap_or("").to_lowercase();
        let parent = row.get_str(2).unwrap_or("");
        if parent.is_empty() {
            continue;
        }
        for obj in state.objects.values_mut() {
            if obj.kind != "indexes" {
                continue;
            }
            if obj.schema.eq_ignore_ascii_case(&schema)
                && obj.name.eq_ignore_ascii_case(&index)
                && obj.parent.as_ref().is_none_or(String::is_empty)
            {
                obj.parent = Some(parent.to_owned());
            }
        }
    }
    Ok(())
}
