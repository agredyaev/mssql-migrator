use super::arena::LayoutArena;
use super::key::ObjectKey;
use super::shared::SharedStr;
use super::str_off::StrOff;
use super::Workspace;

impl Workspace {
    /// Returns the `LayoutArena` for this workspace; panics if strings have not been interned.
    pub fn layout_arena(&self) -> &LayoutArena {
        match self.layout_arena.as_ref() {
            Some(arena) => arena,
            None => panic!("layout arena missing; call intern_workspace_strings first"),
        }
    }

    /// Returns the string slice at the given `StrOff` from the layout arena.
    pub fn str_at(&self, off: StrOff) -> &str {
        let (o, len) = (off.0, off.1);
        self.layout_arena().str_at(o, len)
    }

    /// Returns a `SharedStr` backed by the arena slice at `off`.
    pub fn shared_at(&self, off: StrOff) -> SharedStr {
        let (o, len) = (off.0, off.1);
        self.layout_arena().shared_at(o, len)
    }

    /// Resolves an `ObjectKey` from a layout `StrOff`.
    pub fn object_key(&self, key_off: StrOff) -> ObjectKey {
        ObjectKey::from(self.shared_at(key_off))
    }
}

impl StrOff {
    /// The zero-length offset sentinel (offset 0, length 0).
    pub const EMPTY: StrOff = StrOff(0, 0);

    /// Creates a `StrOff` for the given string already present in `arena`.
    pub fn from_arena(arena: &LayoutArena, s: &str) -> Self {
        let (off, len) = arena.offset_len(s);
        Self(off, len)
    }
}
