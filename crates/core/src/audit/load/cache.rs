#![allow(missing_docs)]
//! Process-local audit caches: ensured-DB set, history-empty/nonempty probes.
//!
//! ### Purpose
//! Caches per-database results of audit-table existence checks and history
//! emptiness probes so that repeated `rmig` operations skip redundant SQL
//! round-trips within the same process lifetime.
//!
//! ### Non-obvious behaviour
//! - All accessors recover poisoned `std::sync::Mutex` values. A prior panic is
//!   logged, but later commands in the same process can still re-check SQL
//!   state instead of crashing before any database access.
//! - `invalidate_audit_cache_all` drops the bootstrap-ensured flag as well as
//!   history probes, forcing a full re-check on the next access.

use std::collections::{HashMap, HashSet};
use std::sync::{Mutex, MutexGuard, OnceLock};

static ENSURED_DBS: OnceLock<Mutex<HashSet<String>>> = OnceLock::new();
static HISTORY_EMPTY: OnceLock<Mutex<HashMap<String, bool>>> = OnceLock::new();

fn ensured_dbs() -> &'static Mutex<HashSet<String>> {
    ENSURED_DBS.get_or_init(|| Mutex::new(HashSet::new()))
}

fn history_nonempty_cache() -> &'static Mutex<HashSet<String>> {
    static HISTORY_NONEMPTY: OnceLock<Mutex<HashSet<String>>> = OnceLock::new();
    HISTORY_NONEMPTY.get_or_init(|| Mutex::new(HashSet::new()))
}

fn history_empty_cache() -> &'static Mutex<HashMap<String, bool>> {
    HISTORY_EMPTY.get_or_init(|| Mutex::new(HashMap::new()))
}

fn lock_cache<'a, T>(cache: &'a Mutex<T>, cache_name: &'static str) -> MutexGuard<'a, T> {
    match cache.lock() {
        Ok(guard) => guard,
        Err(poisoned) => {
            tracing::warn!(
                cache = cache_name,
                "process-local audit cache mutex was poisoned; recovering cached state"
            );
            poisoned.into_inner()
        }
    }
}

pub fn tables_ensured(db_fp: &str) -> bool {
    lock_cache(ensured_dbs(), "ensured_dbs").contains(db_fp)
}

pub fn mark_tables_ensured(db_fp: &str) {
    lock_cache(ensured_dbs(), "ensured_dbs").insert(db_fp.to_string());
}

pub fn history_known_empty(db_fp: &str) -> bool {
    lock_cache(history_empty_cache(), "history_empty")
        .get(db_fp)
        .is_some_and(|&v| v)
}

pub fn history_known_nonempty(db_fp: &str) -> bool {
    lock_cache(history_nonempty_cache(), "history_nonempty").contains(db_fp)
}

pub fn history_empty_cached(db_fp: &str) -> Option<bool> {
    lock_cache(history_empty_cache(), "history_empty")
        .get(db_fp)
        .copied()
}

pub fn cache_history_empty(db_fp: &str, empty: bool) {
    lock_cache(history_empty_cache(), "history_empty").insert(db_fp.to_string(), empty);
    if !empty {
        mark_history_nonempty(db_fp);
    }
}

pub fn mark_history_nonempty(db_fp: &str) {
    lock_cache(history_nonempty_cache(), "history_nonempty").insert(db_fp.to_string());
    lock_cache(history_empty_cache(), "history_empty").insert(db_fp.to_string(), false);
}

pub fn db_fingerprint(server: &str, port: &str, user: &str, database: &str) -> String {
    // Length-prefix the server so `server="s1"/db="a_b"` cannot collide with
    // `server="s1_a"/db="b"` (both were previously `s1_a_b`). Safe as a cache
    // key and as an L1 directory name.
    //
    // Port and user are part of the identity: two instances on one host that
    // differ only by port serve different catalogs, and SQL Server metadata
    // visibility is principal-dependent — neither may share cached plan state.
    format!("{}~{server}~{port}~{user}~{database}", server.len())
}

/// Drop cached history probes (after audit writes). Does not clear bootstrap cache.
pub fn invalidate_audit_cache(db_fp: &str) {
    lock_cache(history_empty_cache(), "history_empty").remove(db_fp);
    lock_cache(history_nonempty_cache(), "history_nonempty").remove(db_fp);
}

/// Full process-local audit cache drop (e.g. after DROP/CREATE test database).
pub fn invalidate_audit_cache_all(db_fp: &str) {
    invalidate_audit_cache(db_fp);
    lock_cache(ensured_dbs(), "ensured_dbs").remove(db_fp);
}

#[cfg(test)]
#[path = "../../tests/cache_identity_test.rs"]
mod cache_identity_tests;

#[cfg(test)]
mod tests {
    use std::panic;
    use std::sync::Mutex;

    use super::lock_cache;

    #[test]
    fn lock_cache_recovers_poisoned_mutex_regression() {
        let cache = Mutex::new(41usize);
        let _ = panic::catch_unwind(|| {
            let _guard = cache.lock().expect("test lock");
            panic!("poison test cache");
        });

        let mut guard = lock_cache(&cache, "test_cache");
        *guard += 1;

        assert_eq!(*guard, 42);
    }
}
