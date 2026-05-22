use std::collections::HashMap;
use std::hash::{Hash, Hasher};
use std::sync::{Mutex, OnceLock};

use crate::db::state::CatalogState;

static INSPECT_CACHE: OnceLock<Mutex<HashMap<String, CatalogState>>> = OnceLock::new();

fn inspect_cache() -> &'static Mutex<HashMap<String, CatalogState>> {
    INSPECT_CACHE.get_or_init(|| Mutex::new(HashMap::new()))
}

fn cache_key(db_fp: &str, layout_digest: &[u8; 32], scope_json: &str) -> String {
    let mut h = std::collections::hash_map::DefaultHasher::new();
    db_fp.hash(&mut h);
    layout_digest.hash(&mut h);
    scope_json.hash(&mut h);
    format!("{db_fp}:{:x}", h.finish())
}

pub fn try_get(db_fp: &str, layout_digest: &[u8; 32], scope_json: &str) -> Option<CatalogState> {
    let key = cache_key(db_fp, layout_digest, scope_json);
    let state = inspect_cache().lock().unwrap().get(&key).cloned()?;
    if state.objects.is_empty() && state.schemas.is_empty() {
        return None;
    }
    Some(state)
}

pub fn store(db_fp: &str, layout_digest: &[u8; 32], scope_json: &str, state: &CatalogState) {
    let key = cache_key(db_fp, layout_digest, scope_json);
    inspect_cache().lock().unwrap().insert(key, state.clone());
}

pub fn invalidate_db(db_fp: &str) {
    inspect_cache()
        .lock()
        .unwrap()
        .retain(|k: &String, _| !k.starts_with(db_fp));
}
