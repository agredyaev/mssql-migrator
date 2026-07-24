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

/// Creates scaffold migration files for all blocked objects that lack non-scaffold scripts.
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
        let db = obj.database_name.as_str();
        if db.is_empty() {
            continue;
        }
        let mig_dir = dir::migration_dir_checked(
            base,
            db,
            obj.schema_name.as_str(),
            obj.object_name.as_str(),
        )?;
        if dir::has_non_scaffold_sql(&mig_dir) {
            continue;
        }
        let cols = columns
            .get(obj.normalized_key.as_str())
            .map(|v| v.as_slice())
            .unwrap_or(&[]);
        let (file_name, content) = pick_content(ws, obj, cols, &commit, &mig_dir)?;
        let path = mig_dir.join(&file_name);
        if path.exists() {
            continue;
        }
        match dir::write_file(&path, &content) {
            Ok(()) => created = true,
            Err(e) if e.kind() == std::io::ErrorKind::AlreadyExists => continue,
            Err(e) => return Err(Error::Io(e)),
        }
    }
    Ok(created)
}

fn pick_content(
    ws: &Workspace,
    obj: &crate::export::PlannedObject,
    cols: &[TableColumn],
    commit: &str,
    mig_dir: &Path,
) -> Result<(String, String)> {
    let lookup_key = ObjectKey::from_normalized(obj.normalized_key.as_str());
    if let Some(entry) = ws.object_by_key(&lookup_key) {
        let script = ws.script(entry.script_id);
        match crate::file_io::read_bounded(
            Path::new(script.abs_path()),
            crate::file_io::MAX_SQL_SCRIPT_BYTES,
        ) {
            Ok(data) => {
                let Ok(data) = String::from_utf8(data) else {
                    return Ok(auto::fallback_scaffold(obj, cols, commit));
                };
                if let Some((content, name)) =
                    auto::try_auto_migration(obj, &data, cols, commit, mig_dir)
                {
                    return Ok((name, content));
                }
            }
            Err(e) if e.kind() == std::io::ErrorKind::InvalidData => return Err(Error::Io(e)),
            Err(_) => {}
        }
    }
    Ok(auto::fallback_scaffold(obj, cols, commit))
}
