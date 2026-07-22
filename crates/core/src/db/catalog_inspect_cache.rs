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
//! - Uses a process-global `LazyLock<Mutex<HashMap<...>>>`. The accessor recovers
//!   poisoned mutexes and logs the event so one panicked worker does not crash
//!   all later commands in the same process.

use std::collections::HashMap;
use std::hash::{Hash, Hasher};
use std::sync::{LazyLock, Mutex, MutexGuard};

use crate::db::state::CatalogState;
use crate::driver::io_profile::lock_unpoisoned;

static INSPECT_CACHE: LazyLock<Mutex<HashMap<String, CatalogState>>> =
    LazyLock::new(|| Mutex::new(HashMap::new()));

fn lock_inspect_cache() -> MutexGuard<'static, HashMap<String, CatalogState>> {
    if INSPECT_CACHE.is_poisoned() {
        tracing::warn!(
            cache = "catalog_inspect",
            "process-local catalog inspect cache mutex was poisoned; recovering cached state"
        );
    }
    lock_unpoisoned(&INSPECT_CACHE)
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
