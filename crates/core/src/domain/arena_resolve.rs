use super::arena::LayoutArena;
use super::key::{ObjectKey, ScriptKey};
use super::key_fingerprint;
use super::shared::SharedStr;
use super::str_off::StrOff;
use super::Workspace;

impl Workspace {
    pub fn layout_arena(&self) -> &LayoutArena {
        match self.layout_arena.as_ref() {
            Some(arena) => arena,
            None => panic!("layout arena missing; call intern_workspace_strings first"),
        }
    }

    pub fn str_at(&self, off: StrOff) -> &str {
        let (o, len) = (off.0, off.1);
        self.layout_arena().str_at(o, len)
    }

    pub fn shared_at(&self, off: StrOff) -> SharedStr {
        let (o, len) = (off.0, off.1);
        self.layout_arena().shared_at(o, len)
    }

    pub fn object_key(&self, key_off: StrOff) -> ObjectKey {
        ObjectKey::from(self.shared_at(key_off))
    }

    pub fn script_key(&self, path_off: StrOff) -> ScriptKey {
        ScriptKey::from(self.shared_at(path_off))
    }

    /// `ChecksumMap` lookup key from layout `key_off` (no `ObjectKey` alloc; fingerprint via [`super::key_fingerprint`]).
    pub fn key_off_fingerprint(&self, off: StrOff) -> u64 {
        let (o, len) = (off.0, off.1);
        key_fingerprint(self.layout_arena().slice_bytes(o, len))
    }
}

impl StrOff {
    pub const EMPTY: StrOff = StrOff(0, 0);

    pub fn new(off: u32, len: u32) -> Self {
        Self(off, len)
    }

    pub fn from_arena(arena: &LayoutArena, s: &str) -> Self {
        let (off, len) = arena.offset_len(s);
        Self(off, len)
    }
}
