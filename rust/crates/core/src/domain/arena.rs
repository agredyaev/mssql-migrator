use std::collections::HashMap;
use std::sync::Arc;

use super::key::{ObjectKey, ScriptKey};
use super::shared::SharedStr;

/// Single backing buffer for deduplicated strings (Kelley / Go `stringArena`).
pub struct StringArenaBuilder {
    buf: Vec<u8>,
    index: HashMap<Box<str>, u32>,
}

pub struct StringArena {
    buf: Arc<[u8]>,
    index: HashMap<Box<str>, u32>,
}

impl StringArenaBuilder {
    pub fn with_capacity(byte_hint: usize, unique_hint: usize) -> Self {
        Self {
            buf: Vec::with_capacity(byte_hint),
            index: HashMap::with_capacity(unique_hint),
        }
    }

    pub fn register(&mut self, s: &str) {
        if s.is_empty() || self.index.contains_key(s) {
            return;
        }
        let off = self.buf.len() as u32;
        self.buf.extend_from_slice(s.as_bytes());
        self.index.insert(s.into(), off);
    }

    pub fn finish(self) -> StringArena {
        StringArena {
            buf: Arc::from(self.buf.into_boxed_slice()),
            index: self.index,
        }
    }
}

impl StringArena {
    pub fn get(&self, s: &str) -> SharedStr {
        if s.is_empty() {
            return SharedStr::empty();
        }
        let off = self
            .index
            .get(s)
            .copied()
            .unwrap_or_else(|| panic!("arena missing string: {s:?}"));
        SharedStr::from_arena_slice(self.buf.clone(), off, s.len() as u32)
    }

    pub fn unique_count(&self) -> usize {
        self.index.len()
    }

    pub fn byte_len(&self) -> usize {
        self.buf.len()
    }
}

/// Arc-dedup interner for synthetic benches (pre-finalize); scan uses [`StringArena`].
pub struct StringInterner {
    index: HashMap<Box<str>, SharedStr>,
}

impl StringInterner {
    pub fn with_capacity(unique_hint: usize) -> Self {
        Self {
            index: HashMap::with_capacity(unique_hint),
        }
    }

    pub fn intern(&mut self, s: &str) -> SharedStr {
        if s.is_empty() {
            return SharedStr::empty();
        }
        if let Some(existing) = self.index.get(s) {
            return existing.clone();
        }
        let shared = SharedStr::new(s);
        self.index.insert(s.into(), shared.clone());
        shared
    }

    pub fn unique_count(&self) -> usize {
        self.index.len()
    }

    pub fn byte_len(&self) -> usize {
        self.index.keys().map(|k| k.len()).sum()
    }
}

pub fn intern_workspace_strings(ws: &mut crate::domain::Workspace) {
    let n = ws.object_entries.len();
    let mut builder =
        StringArenaBuilder::with_capacity(n * 48, n / 4 + ws.scripts.len() + ws.schemas.len() + 1);
    register_workspace_strings(ws, &mut builder);
    let arena = builder.finish();
    apply_workspace_strings(ws, &arena);
    ws.string_arena_bytes = arena.byte_len();
    ws.string_arena_unique = arena.unique_count();
}

fn register_workspace_strings(ws: &crate::domain::Workspace, b: &mut StringArenaBuilder) {
    for obj in &ws.object_entries {
        b.register(obj.schema.as_ref());
        b.register(obj.kind.as_ref());
        b.register(obj.name.as_ref());
        b.register(obj.database_name.as_ref());
        if !obj.parent_name.is_empty() {
            b.register(obj.parent_name.as_ref());
        }
        b.register(obj.key.as_str());
        b.register(obj.script.as_str());
        if let Some(parent) = &obj.parent_key {
            b.register(parent.as_str());
        }
    }
    for script in ws.scripts.values() {
        b.register(script.schema.as_ref());
        b.register(script.object_kind.as_ref());
        b.register(script.object_name.as_ref());
        b.register(script.key.as_str());
        b.register(script.abs_path.as_ref());
        if !script.git_hash.is_empty() {
            b.register(script.git_hash.as_ref());
        }
        if !script.git_author.is_empty() {
            b.register(script.git_author.as_ref());
        }
        if !script.git_date.is_empty() {
            b.register(script.git_date.as_ref());
        }
    }
    for (key, entries) in &ws.transitions_by_table {
        b.register(key.as_str());
        for (path, sk) in entries {
            b.register(path.as_ref());
            b.register(sk.as_str());
        }
    }
    for schema in &ws.schemas {
        b.register(schema.database.as_ref());
        b.register(schema.name.as_ref());
        b.register(schema.normalized.as_ref());
    }
}

fn apply_workspace_strings(ws: &mut crate::domain::Workspace, arena: &StringArena) {
    for obj in &mut ws.object_entries {
        obj.schema = arena.get(obj.schema.as_ref());
        obj.kind = arena.get(obj.kind.as_ref());
        obj.name = arena.get(obj.name.as_ref());
        obj.database_name = arena.get(obj.database_name.as_ref());
        if !obj.parent_name.is_empty() {
            obj.parent_name = arena.get(obj.parent_name.as_ref());
        }
        let new_key = ObjectKey::from(arena.get(obj.key.as_str()));
        obj.key = new_key.clone();
        obj.script = ScriptKey::from(arena.get(obj.script.as_str()));
        if let Some(parent) = obj.parent_key.take() {
            obj.parent_key = Some(ObjectKey::from(arena.get(parent.as_str())));
        }
    }

    for script in ws.scripts.values_mut() {
        script.schema = arena.get(script.schema.as_ref());
        script.object_kind = arena.get(script.object_kind.as_ref());
        script.object_name = arena.get(script.object_name.as_ref());
        script.key = ScriptKey::from(arena.get(script.key.as_str()));
        script.abs_path = arena.get(script.abs_path.as_ref());
        if !script.git_hash.is_empty() {
            script.git_hash = arena.get(script.git_hash.as_ref());
        }
        if !script.git_author.is_empty() {
            script.git_author = arena.get(script.git_author.as_ref());
        }
        if !script.git_date.is_empty() {
            script.git_date = arena.get(script.git_date.as_ref());
        }
    }

    let transitions = std::mem::take(&mut ws.transitions_by_table);
    ws.transitions_by_table = HashMap::with_capacity(transitions.len());
    for (key, entries) in transitions {
        let new_key = ObjectKey::from(arena.get(key.as_str()));
        let new_entries = entries
            .into_iter()
            .map(|(path, sk)| {
                (
                    arena.get(path.as_ref()),
                    ScriptKey::from(arena.get(sk.as_str())),
                )
            })
            .collect();
        ws.transitions_by_table.insert(new_key, new_entries);
    }

    ws.invalidate_transition_paths();

    for schema in ws.schemas.iter_mut() {
        schema.database = arena.get(schema.database.as_ref());
        schema.name = arena.get(schema.name.as_ref());
        schema.normalized = arena.get(schema.normalized.as_ref());
    }
}

/// Kelley buffer for git metadata applied after [`super::git_preload::preload`].
pub fn intern_script_git_strings(ws: &mut crate::domain::Workspace) {
    let has_git = ws.scripts.values().any(|s| {
        !s.git_hash.is_empty() || !s.git_author.is_empty() || !s.git_date.is_empty()
    });
    if !has_git {
        return;
    }
    let mut builder = StringArenaBuilder::with_capacity(ws.scripts.len() * 48, ws.scripts.len());
    for script in ws.scripts.values() {
        if !script.git_hash.is_empty() {
            builder.register(script.git_hash.as_ref());
        }
        if !script.git_author.is_empty() {
            builder.register(script.git_author.as_ref());
        }
        if !script.git_date.is_empty() {
            builder.register(script.git_date.as_ref());
        }
    }
    let arena = builder.finish();
    for script in ws.scripts.values_mut() {
        if !script.git_hash.is_empty() {
            script.git_hash = arena.get(script.git_hash.as_ref());
        }
        if !script.git_author.is_empty() {
            script.git_author = arena.get(script.git_author.as_ref());
        }
        if !script.git_date.is_empty() {
            script.git_date = arena.get(script.git_date.as_ref());
        }
    }
    ws.string_arena_bytes = ws.string_arena_bytes.saturating_add(arena.byte_len());
    ws.string_arena_unique = ws.string_arena_unique.saturating_add(arena.unique_count());
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn dedups_repeated_strings() {
        let mut interner = StringInterner::with_capacity(4);
        let a = interner.intern("schema");
        let b = interner.intern("schema");
        assert_eq!(a, b);
        assert_eq!(interner.unique_count(), 1);
    }

    #[test]
    fn arena_single_buffer() {
        let mut b = StringArenaBuilder::with_capacity(32, 4);
        b.register("schema");
        b.register("views");
        b.register("schema");
        let arena = b.finish();
        assert_eq!(arena.unique_count(), 2);
        assert_eq!(arena.byte_len(), "schema".len() + "views".len());
        assert_eq!(arena.get("schema"), arena.get("schema"));
    }
}
