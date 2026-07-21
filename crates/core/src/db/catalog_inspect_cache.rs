//! Process-local catalog inspect cache keyed by `(db_fp, layout_digest, scope_json)`.
//!
//! ### Purpose
//! Avoids redundant SQL catalog queries when the same scope has already been
//! inspected within the same process lifetime.
//!
//! ### Non-obvious
//! - `try_get_unless_bypassed` returns `None` when the cached state is empty
//!   (both objects and schemas empty) to force a re-inspect rather than
//!   treating an empty catalog as a cache hit.
//! - Uses a process-global `OnceLock<Mutex<HashMap<...>>>`. Accessors recover
//!   poisoned mutexes and log the event so one panicked worker does not crash
//!   all later commands in the same process.

use std::collections::HashMap;
use std::hash::{Hash, Hasher};
use std::sync::{Mutex, MutexGuard, OnceLock};

use crate::db::state::CatalogState;

static INSPECT_CACHE: OnceLock<Mutex<HashMap<String, CatalogState>>> = OnceLock::new();

fn inspect_cache() -> &'static Mutex<HashMap<String, CatalogState>> {
    INSPECT_CACHE.get_or_init(|| Mutex::new(HashMap::new()))
}

fn lock_cache<'a>(
    cache: &'a Mutex<HashMap<String, CatalogState>>,
) -> MutexGuard<'a, HashMap<String, CatalogState>> {
    match cache.lock() {
        Ok(guard) => guard,
        Err(poisoned) => {
            tracing::warn!(
                cache = "catalog_inspect",
                "process-local catalog inspect cache mutex was poisoned; recovering cached state"
            );
            poisoned.into_inner()
        }
    }
}

fn lock_inspect_cache() -> MutexGuard<'static, HashMap<String, CatalogState>> {
    lock_cache(inspect_cache())
}

fn cache_key(db_fp: &str, layout_digest: &[u8; 32], scope_json: &str) -> String {
    let mut h = std::collections::hash_map::DefaultHasher::new();
    db_fp.hash(&mut h);
    layout_digest.hash(&mut h);
    scope_json.hash(&mut h);
    format!("{db_fp}:{:x}", h.finish())
}

pub fn store(db_fp: &str, layout_digest: &[u8; 32], scope_json: &str, state: &CatalogState) {
    let key = cache_key(db_fp, layout_digest, scope_json);
    lock_inspect_cache().insert(key, state.clone());
}

pub fn invalidate_db(db_fp: &str) {
    lock_inspect_cache().retain(|k: &String, _| !k.starts_with(db_fp));
}

/// Catalog inspect cache read honouring the mutating-command cache bypass:
/// mutating plans must re-inspect live catalog state, never a process-local
/// snapshot.
pub fn try_get_unless_bypassed(
    bypass: bool,
    db_fp: &str,
    layout_digest: &[u8; 32],
    scope_json: &str,
) -> Option<CatalogState> {
    if bypass {
        return None;
    }
    let key = cache_key(db_fp, layout_digest, scope_json);
    let state = lock_inspect_cache().get(&key).cloned()?;
    // Empty catalog could mean the database has no objects — a cache hit would
    // cause us to skip inspection permanently. Force re-inspect instead.
    if state.objects.is_empty() && state.schemas.is_empty() {
        return None;
    }
    Some(state)
}

#[cfg(test)]
mod tests {
    use std::collections::HashMap;
    use std::panic;
    use std::sync::Mutex;

    use super::lock_cache;
    use crate::db::state::CatalogState;

    #[test]
    fn lock_cache_recovers_poisoned_mutex_regression() {
        let cache = Mutex::new(HashMap::<String, CatalogState>::new());
        let _ = panic::catch_unwind(|| {
            let _guard = cache.lock().expect("test lock");
            panic!("poison test cache");
        });

        let mut guard = lock_cache(&cache);
        guard.insert("db:scope".into(), CatalogState::default());

        assert!(guard.contains_key("db:scope"));
    }
}
