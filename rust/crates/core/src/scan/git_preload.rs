use std::collections::HashMap;

use crate::domain::{share, Script, ScriptKey, Workspace};

use super::git_log::{self, GitMeta};
use super::git_repo;

pub fn preload(ws: &mut Workspace, sql_root: &str) {
    if !git_repo::has_git_repo(sql_root) {
        return;
    }
    let root = match git_repo::git_work_tree(sql_root) {
        Some(r) => r,
        None => return,
    };
    let prefix = git_repo::sql_path_prefix(sql_root);
    let targets = build_targets(ws);
    if targets.is_empty() {
        return;
    }
    let mut remaining: HashMap<String, ScriptKey> = targets;
    if let Some(out) = git_log::batched_git_log(&root) {
        apply_batched(&out, prefix.as_deref(), &mut remaining, ws);
    }
    for (path, key) in remaining {
        let script = match ws.scripts.get(&key) {
            Some(s) => s,
            None => continue,
        };
        if let Some(meta) = git_log::git_info_file(script.abs_path.as_ref()) {
            apply_meta(ws.scripts.get_mut(&key).unwrap(), &meta);
            let _ = path;
        }
    }
}

fn build_targets(ws: &Workspace) -> HashMap<String, ScriptKey> {
    let mut m = HashMap::new();
    for (key, script) in &ws.scripts {
        m.insert(script.key.as_str().to_string(), key.clone());
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
    for line in std::str::from_utf8(out).unwrap_or("").lines() {
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
        if let Some(s) = ws.scripts.get_mut(&key) {
            apply_meta(s, &meta);
        }
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

fn apply_meta(s: &mut Script, meta: &GitMeta) {
    s.git_hash = share(&meta.hash);
    s.git_author = share(&meta.author);
    s.git_date = share(&meta.date);
}
