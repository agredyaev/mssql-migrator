use std::collections::{HashMap, HashSet};

use serde::{Deserialize, Serialize};

use crate::domain::{share, ObjectKey, SharedStr};

/// Prior checksum digests keyed by normalized object key (no duplicate `String` keys).
pub type ChecksumMap = HashMap<ObjectKey, [u8; 32]>;

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
pub struct CatalogState {
    pub schemas: HashSet<String>,
    pub objects: HashMap<ObjectKey, CatalogObject>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct CatalogObject {
    pub schema: SharedStr,
    pub kind: SharedStr,
    pub name: SharedStr,
    pub parent: Option<SharedStr>,
}

impl CatalogState {
    pub fn exists_key(&self, key: &ObjectKey) -> bool {
        self.objects.contains_key(key)
    }
}

#[derive(Clone, Debug)]
pub struct TableColumn {
    pub name: String,
    pub type_name: String,
    pub nullable: bool,
}

/// Build a catalog row from SQL inspect wire strings (deduped via `share`).
pub fn catalog_object(
    schema: &str,
    kind: &str,
    name: &str,
    parent: Option<&str>,
) -> CatalogObject {
    catalog_object_parts(
        share(schema),
        share(kind),
        share(name),
        parent.map(share),
    )
}

/// Build a catalog row reusing layout `SharedStr` (no duplicate `share()` / Arc).
pub fn catalog_object_parts(
    schema: SharedStr,
    kind: SharedStr,
    name: SharedStr,
    parent: Option<SharedStr>,
) -> CatalogObject {
    CatalogObject {
        schema,
        kind,
        name,
        parent,
    }
}

/// Deduplicate catalog wire strings via domain arena (called after SQL load).
pub fn intern_catalog_state(state: &mut CatalogState) {
    if state.objects.is_empty() && state.schemas.is_empty() {
        return;
    }
    use crate::domain::StringArenaBuilder;
    let mut builder = StringArenaBuilder::with_capacity(
        state.objects.len() * 48 + state.schemas.len() * 16,
        state.objects.len() + state.schemas.len(),
    );
    for schema in &state.schemas {
        builder.register(schema);
    }
    for obj in state.objects.values() {
        builder.register(obj.schema.as_ref());
        builder.register(obj.kind.as_ref());
        builder.register(obj.name.as_ref());
        if let Some(parent) = &obj.parent {
            builder.register(parent.as_ref());
        }
    }
    let arena = builder.finish();
    state.schemas = state
        .schemas
        .iter()
        .map(|s| arena.get(s).as_ref().to_string())
        .collect();
    for obj in state.objects.values_mut() {
        obj.schema = arena.get(obj.schema.as_ref());
        obj.kind = arena.get(obj.kind.as_ref());
        obj.name = arena.get(obj.name.as_ref());
        if let Some(parent) = obj.parent.take() {
            obj.parent = Some(arena.get(parent.as_ref()));
        }
    }
}
