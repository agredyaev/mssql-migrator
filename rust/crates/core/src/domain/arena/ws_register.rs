use super::StringArenaBuilder;
use crate::domain::Workspace;

pub(super) fn register_workspace_strings(ws: &Workspace, b: &mut StringArenaBuilder) {
    for obj in &ws.object_entries {
        b.register(obj.key_str(ws));
        b.register(ws.database_name(obj.db_id).as_ref());
        b.register(obj.script_path(ws));
    }
    for row in ws.script_rows.iter() {
        b.register(row.path_str(ws));
        b.register(row.abs_path(ws).as_ref());
    }
    register_script_git(ws, b);
    for entries in ws.transitions_by_row.values() {
        for e in entries {
            if let Some(ord) = &e.staging_ord {
                b.register(ord.as_ref());
            }
        }
    }
    for schema in &ws.schemas {
        b.register(schema.database.as_ref());
        b.register(schema.name.as_ref());
        b.register(schema.normalized.as_ref());
    }
}

fn register_script_git(ws: &Workspace, b: &mut StringArenaBuilder) {
    for git in ws.script_git.values() {
        if let Some(s) = git.staging_hash.as_ref() {
            b.register(s.as_ref());
        }
        if let Some(s) = git.staging_author.as_ref() {
            b.register(s.as_ref());
        }
        if let Some(s) = git.staging_date.as_ref() {
            b.register(s.as_ref());
        }
    }
}
