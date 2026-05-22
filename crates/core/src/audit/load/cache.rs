use std::collections::{HashMap, HashSet};
use std::sync::{Mutex, OnceLock};

static ENSURED_DBS: OnceLock<Mutex<HashSet<String>>> = OnceLock::new();
static HISTORY_EMPTY: OnceLock<Mutex<HashMap<String, bool>>> = OnceLock::new();

fn ensured_dbs() -> &'static Mutex<HashSet<String>> {
    ENSURED_DBS.get_or_init(|| Mutex::new(HashSet::new()))
}

fn history_nonempty_cache() -> &'static Mutex<HashSet<String>> {
    static HISTORY_NONEMPTY: OnceLock<Mutex<HashSet<String>>> = OnceLock::new();
    HISTORY_NONEMPTY.get_or_init(|| Mutex::new(HashSet::new()))
}

pub(super) fn history_empty_cache() -> &'static Mutex<HashMap<String, bool>> {
    HISTORY_EMPTY.get_or_init(|| Mutex::new(HashMap::new()))
}

pub fn tables_ensured(db_fp: &str) -> bool {
    ensured_dbs().lock().unwrap().contains(db_fp)
}

pub fn mark_tables_ensured(db_fp: &str) {
    ensured_dbs().lock().unwrap().insert(db_fp.to_string());
}

pub fn history_known_empty(db_fp: &str) -> bool {
    history_empty_cache()
        .lock()
        .unwrap()
        .get(db_fp)
        .is_some_and(|&v| v)
}

pub fn history_known_nonempty(db_fp: &str) -> bool {
    history_nonempty_cache().lock().unwrap().contains(db_fp)
}

pub fn history_empty_cached(db_fp: &str) -> Option<bool> {
    history_empty_cache().lock().unwrap().get(db_fp).copied()
}

pub fn cache_history_empty(db_fp: &str, empty: bool) {
    history_empty_cache()
        .lock()
        .unwrap()
        .insert(db_fp.to_string(), empty);
    if !empty {
        mark_history_nonempty(db_fp);
    }
}

pub fn mark_history_nonempty(db_fp: &str) {
    history_nonempty_cache()
        .lock()
        .unwrap()
        .insert(db_fp.to_string());
    history_empty_cache()
        .lock()
        .unwrap()
        .insert(db_fp.to_string(), false);
}

pub fn db_fingerprint(server: &str, database: &str) -> String {
    format!("{server}_{database}")
}

/// Drop cached history probes (after audit writes). Does not clear bootstrap cache.
pub fn invalidate_audit_cache(db_fp: &str) {
    history_empty_cache().lock().unwrap().remove(db_fp);
    history_nonempty_cache().lock().unwrap().remove(db_fp);
}

/// Full process-local audit cache drop (e.g. after DROP/CREATE test database).
pub fn invalidate_audit_cache_all(db_fp: &str) {
    invalidate_audit_cache(db_fp);
    ensured_dbs().lock().unwrap().remove(db_fp);
}
