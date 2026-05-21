use super::StringArena;
use crate::domain::str_off::StrOff;
use crate::domain::Workspace;

pub(super) fn apply_workspace_strings(ws: &mut Workspace, arena: &StringArena) {
    apply_object_entries(ws, arena);
    apply_parents(ws, arena);
    apply_scripts(ws, arena);
    apply_transitions(ws, arena);
    ws.invalidate_transition_paths();
    apply_schemas(ws, arena);
}

fn apply_object_entries(ws: &mut Workspace, arena: &StringArena) {
    let n = ws.object_entries.len();
    let mut key_strs: Vec<String> = Vec::with_capacity(n);
    let mut db_names: Vec<String> = Vec::with_capacity(n);
    for obj in &ws.object_entries {
        db_names.push(ws.database_name(obj.db_id).as_ref().to_string());
        key_strs.push(
            obj.staging_key
                .as_ref()
                .expect("staging_key before intern")
                .as_str()
                .to_string(),
        );
    }
    ws.database_names.clear();
    ws.database_names.push(crate::domain::empty_str());
    for i in 0..n {
        let db = arena.get(db_names[i].as_str());
        let key_off = StrOff::from_arena(arena, key_strs[i].as_str());
        ws.object_entries[i].db_id = ws.intern_database(db);
        ws.object_entries[i].key_off = key_off;
        ws.object_entries[i].staging_key = None;
    }
}

fn apply_parents(_ws: &mut Workspace, _arena: &StringArena) {
    // ParentRef is row-id only (**IDX**); no string fields to intern.
}

fn apply_scripts(ws: &mut Workspace, arena: &StringArena) {
    for row in ws.script_rows.iter_mut() {
        let key_str = row
            .staging_key
            .as_ref()
            .expect("staging_key before intern")
            .as_str();
        let abs_str = row
            .staging_abs_path
            .as_ref()
            .expect("staging_abs_path before intern")
            .as_ref();
        row.path_off = StrOff::from_arena(arena, key_str);
        row.abs_path_off = StrOff::from_arena(arena, abs_str);
        row.staging_key = None;
        row.staging_abs_path = None;
    }
}

fn apply_transitions(ws: &mut Workspace, arena: &StringArena) {
    for entries in ws.transitions_by_row.values_mut() {
        for e in entries.iter_mut() {
            if let Some(ord) = e.staging_ord.take() {
                e.ord_off = StrOff::from_arena(arena, ord.as_ref());
            }
        }
    }
}

fn apply_schemas(ws: &mut Workspace, arena: &StringArena) {
    for schema in ws.schemas.iter_mut() {
        schema.database = arena.get(schema.database.as_ref());
        schema.name = arena.get(schema.name.as_ref());
        schema.normalized = arena.get(schema.normalized.as_ref());
    }
}
