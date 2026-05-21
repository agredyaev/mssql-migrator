use std::collections::HashMap;
use std::path::Path;

use crate::config::Config;
use crate::db::TableColumn;
use crate::domain::{Action, ObjectKey, Workspace};
use crate::error::{Error, Result};
use crate::export::MigrationPlan;

use super::auto;
use super::dir;
use super::git;

pub fn ensure(
    cfg: &Config,
    ws: &Workspace,
    plan: &MigrationPlan,
    columns: &HashMap<String, Vec<TableColumn>>,
) -> Result<bool> {
    let base = Path::new(&cfg.sql_base);
    let commit = git::short_hash();
    let mut created = false;
    for obj in &plan.objects {
        if obj.planned_action != Action::ReprocessChangedBlocked {
            continue;
        }
        let db = obj.database_name.as_ref();
        if db.is_empty() {
            continue;
        }
        let mig_dir = dir::migration_dir_checked(
            base,
            db,
            obj.schema_name.as_ref(),
            obj.object_name.as_ref(),
        )?;
        if dir::has_non_scaffold_sql(&mig_dir) {
            continue;
        }
        let cols = columns
            .get(obj.normalized_key.as_ref())
            .map(|v| v.as_slice())
            .unwrap_or(&[]);
        let (file_name, content) = pick_content(ws, obj, cols, &commit, &mig_dir);
        let path = mig_dir.join(&file_name);
        if path.exists() {
            continue;
        }
        dir::write_file(&path, &content).map_err(Error::Io)?;
        created = true;
    }
    Ok(created)
}

fn pick_content(
    ws: &Workspace,
    obj: &crate::export::PlannedObject,
    cols: &[TableColumn],
    commit: &str,
    mig_dir: &Path,
) -> (String, String) {
    let lookup_key = ObjectKey::from(obj.normalized_key.as_ref().to_string());
    if let Some(entry) = ws.object_by_key(&lookup_key) {
        let script = ws.script(entry.script_id);
        if let Ok(data) = std::fs::read_to_string(script.abs_path().as_ref()) {
            if let Some((content, name)) =
                auto::try_auto_migration(obj, &data, cols, commit, mig_dir)
            {
                return (name, content);
            }
        }
    }
    auto::fallback_scaffold(obj, cols, commit)
}
