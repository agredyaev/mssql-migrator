use super::StringArena;
use crate::domain::str_off::StrOff;
use crate::domain::Workspace;

pub(super) fn apply_scripts(ws: &mut Workspace, arena: &StringArena) {
    let n = ws.script_rows.len();
    if n == 0 {
        return;
    }
    let mut fast = true;
    let mut path_offs = Vec::with_capacity(n);
    let mut abs_offs = Vec::with_capacity(n);
    for i in 0..n {
        let key = &ws.cold.ingest_script_keys[i];
        let abs = &ws.cold.ingest_script_abs[i];
        let key_s = key.shared();
        let abs_s = abs.clone();
        let Some(path_off) = arena.str_off_if_dedicated(&key_s) else {
            fast = false;
            break;
        };
        let Some(abs_off) = arena.str_off_if_dedicated(&abs_s) else {
            fast = false;
            break;
        };
        path_offs.push(path_off);
        abs_offs.push(abs_off);
    }
    if fast && path_offs.len() == n {
        for (row, (&path_off, &abs_off)) in ws
            .script_rows
            .iter_mut()
            .zip(path_offs.iter().zip(abs_offs.iter()))
        {
            row.path_off = path_off;
            row.abs_path_off = abs_off;
        }
        ws.cold.ingest_script_keys.clear();
        ws.cold.ingest_script_abs.clear();
        return;
    }
    let key_strs: Vec<String> = ws
        .cold
        .ingest_script_keys
        .iter()
        .map(|k| k.as_str().to_string())
        .collect();
    let abs_strs: Vec<String> = ws
        .cold
        .ingest_script_abs
        .iter()
        .map(|s| s.as_ref().to_string())
        .collect();
    for (i, row) in ws.script_rows.iter_mut().enumerate() {
        row.path_off = StrOff::from_arena(arena, &key_strs[i]);
        row.abs_path_off = StrOff::from_arena(arena, &abs_strs[i]);
    }
    ws.cold.ingest_script_keys.clear();
    ws.cold.ingest_script_abs.clear();
}
