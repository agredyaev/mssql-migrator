use super::StringArenaBuilder;
use crate::domain::Workspace;

pub(super) fn register_workspace_strings(ws: &Workspace, b: &mut StringArenaBuilder) {
    for (i, obj) in ws.object_entries.iter().enumerate() {
        b.register(obj.key_str(ws, i));
        b.register(ws.database_name(obj.db_id).as_ref());
        b.register(obj.script_path(ws));
    }
    for (i, row) in ws.script_rows.iter().enumerate() {
        let id = (i + 1) as u32;
        b.register(row.path_str(ws, id));
        b.register(row.abs_path(ws, id).as_ref());
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
    for st in ws.cold.script_git_staging.values() {
        if let Some(s) = st.hash.as_ref() {
            b.register(s.as_ref());
        }
        if let Some(s) = st.author.as_ref() {
            b.register(s.as_ref());
        }
        if let Some(s) = st.date.as_ref() {
            b.register(s.as_ref());
        }
    }
}
