//! Apply-time script loading with checksum re-verification.

use crate::domain::{path_lookup_candidates, ObjectKey, ScriptKey, Workspace};
use crate::export::PlannedObject;

/// Loads the object's script body, re-verified against the planned checksum.
pub(super) fn read_script(
    ws: &Workspace,
    obj: &PlannedObject,
) -> std::result::Result<String, String> {
    let Some(script) = find_script(ws, obj) else {
        return Err(format!("{}: script not found", obj.normalized_key));
    };
    let expected = script.checksum().copied().unwrap_or(obj.checksum);
    verified_body(script.abs_path(), &expected, &obj.normalized_key)
}

/// Re-read and re-hash the script at apply time: the plan (and the history row
/// about to be written) carries the scan-time checksum, so bytes that changed
/// between scan and apply must abort before any SQL executes.
pub(super) fn verified_body(
    path: &str,
    expected: &[u8; 32],
    label: &str,
) -> std::result::Result<String, String> {
    let data = crate::file_io::read_bounded(
        std::path::Path::new(path),
        crate::file_io::MAX_SQL_SCRIPT_BYTES,
    )
    .map_err(|e| format!("{label}: read failed: {e}"))?;
    if crate::scan::content_checksum(&data) != *expected {
        return Err(format!(
            "{label}: file changed after scan; aborting before execution"
        ));
    }
    String::from_utf8(data).map_err(|_| format!("{label}: script is not valid UTF-8"))
}

fn find_script<'a>(ws: &'a Workspace, obj: &PlannedObject) -> Option<crate::domain::ScriptRef<'a>> {
    for path in path_lookup_candidates(obj.database_name.as_ref(), obj.object_path.as_ref()) {
        if let Some(script) = ws.script_by_key(&ScriptKey::from_path(&path)) {
            return Some(script);
        }
    }
    ws.scripts_iter().find(|s| {
        ObjectKey::parse(s.path_str())
            .map(|k| k.as_str() == obj.normalized_key.as_str())
            .unwrap_or(false)
    })
}

#[cfg(test)]
#[path = "../tests/apply_script_read_test.rs"]
mod apply_script_read_tests;
