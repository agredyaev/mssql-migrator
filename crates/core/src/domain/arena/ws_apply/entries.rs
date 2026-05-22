use super::StringArena;
use crate::domain::str_off::StrOff;
use crate::domain::Workspace;

pub(super) fn apply_object_entries(ws: &mut Workspace, arena: &StringArena) {
    let n = ws.object_entries.len();
    if n == 0 {
        return;
    }
    let mut key_offs = Vec::with_capacity(n);
    let mut db_offs = Vec::with_capacity(n);
    let mut fast = true;
    for (i, obj) in ws.object_entries.iter().enumerate() {
        let key = ws.entry_key(i);
        let key_s = key.shared();
        let Some(key_off) = arena.str_off_if_dedicated(&key_s) else {
            fast = false;
            break;
        };
        let db = ws.database_name(obj.db_id);
        let Some(db_off) = arena.str_off_if_dedicated(&db) else {
            fast = false;
            break;
        };
        key_offs.push(key_off);
        db_offs.push(db_off);
    }
    if fast && key_offs.len() == n {
        ws.database_names.clear();
        ws.database_names.push(crate::domain::empty_str());
        for i in 0..n {
            ws.object_entries[i].db_id =
                ws.intern_database(arena.shared_at(db_offs[i].0, db_offs[i].1));
            ws.object_entries[i].key_off = key_offs[i];
        }
        ws.cold.ingest_keys.clear();
        return;
    }
    let mut key_strs: Vec<String> = Vec::with_capacity(n);
    let mut db_names: Vec<String> = Vec::with_capacity(n);
    for (i, obj) in ws.object_entries.iter().enumerate() {
        db_names.push(ws.database_name(obj.db_id).as_ref().to_string());
        key_strs.push(ws.entry_key(i).as_str().to_string());
    }
    ws.database_names.clear();
    ws.database_names.push(crate::domain::empty_str());
    for i in 0..n {
        let db = arena.get(db_names[i].as_str());
        ws.object_entries[i].db_id = ws.intern_database(db);
        ws.object_entries[i].key_off = StrOff::from_arena(arena, key_strs[i].as_str());
    }
    ws.cold.ingest_keys.clear();
}
