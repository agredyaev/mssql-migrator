use std::collections::HashMap;

use crate::domain::{share, ScriptKey, Workspace};

use crate::git::{self as git_log, GitMeta};

pub fn preload(ws: &mut Workspace, sql_root: &str) {
    if !crate::git::has_git_repo(sql_root) {
        return;
    }
    let root = match git_work_tree(sql_root) {
        Some(r) => r,
        None => return,
    };
    let prefix = crate::git::sql_path_prefix(sql_root);
    let targets = build_targets(ws);
    if targets.is_empty() {
        return;
    }
    let mut remaining: HashMap<String, ScriptKey> = targets;
    if let Some(out) = git_log::batched_git_log(&root) {
        apply_batched(&out, prefix.as_deref(), &mut remaining, ws);
    }
    for (path, key) in remaining {
        let Some(&id) = ws.script_key_index.get(&key) else {
            continue;
        };
        let script = ws.script(id);
        if let Some(meta) = git_log::git_info_file(script.abs_path().as_ref()) {
            apply_meta(ws, id, &meta);
            let _ = path;
        }
    }
}

fn build_targets(ws: &Workspace) -> HashMap<String, ScriptKey> {
    let mut m = HashMap::new();
    for script in ws.scripts_iter() {
        m.insert(script.path_str().to_string(), script.key());
    }
    m
}

fn apply_batched(
    out: &[u8],
    prefix: Option<&str>,
    remaining: &mut HashMap<String, ScriptKey>,
    ws: &mut Workspace,
) {
    let mut cur: Option<GitMeta> = None;
    // Lossy decode: one non-UTF-8 byte (e.g. a latin-1 author name) must not
    // discard the whole git-log batch and force per-script fallback spawns.
    let text = String::from_utf8_lossy(out);
    for line in text.lines() {
        let line = line.trim();
        if line.is_empty() {
            continue;
        }
        if let Some(meta) = git_log::parse_commit_line(line) {
            cur = Some(meta);
            continue;
        }
        let rel = layout_key_for_git_line(line, prefix);
        let Some(key) = remaining.remove(&rel) else {
            continue;
        };
        let Some(meta) = cur.clone() else {
            continue;
        };
        let Some(&id) = ws.script_key_index.get(&key) else {
            continue;
        };
        apply_meta(ws, id, &meta);
    }
}

fn layout_key_for_git_line(line: &str, prefix: Option<&str>) -> String {
    let p = git_log::normalize_git_path(line);
    if let Some(pref) = prefix {
        if let Some(rest) = p.strip_prefix(pref) {
            return rest.to_string();
        }
    }
    p
}

fn apply_meta(ws: &mut Workspace, script_id: u32, meta: &GitMeta) {
    let st = ws.ensure_script_git_staging(script_id);
    st.hash = Some(share(&meta.hash));
    st.author = Some(share(&meta.author));
    st.date = Some(share(&meta.date));
    let _ = ws.ensure_script_git(script_id);
}

fn git_work_tree(sql_root: &str) -> Option<String> {
    std::path::Path::new(sql_root)
        .canonicalize()
        .ok()
        .map(|p| p.to_string_lossy().into_owned())
}
