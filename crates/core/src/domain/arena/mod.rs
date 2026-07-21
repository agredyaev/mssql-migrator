use std::collections::HashMap;

mod builder;
mod slice;
mod ws_apply;
mod ws_git;
mod ws_intern;
mod ws_register;

pub use ws_git::intern_script_git_strings;
pub use ws_intern::{install_layout_arena, intern_workspace_strings};

/// Layout string buffer for scan-finalize deduplication.
pub type LayoutArena = StringArena;

/// Single backing buffer for deduplicated strings.
/// Builder that accumulates strings into a contiguous byte buffer before sealing as a `StringArena`.
pub struct StringArenaBuilder {
    pub(super) buf: Vec<u8>,
    pub(super) index: HashMap<Box<str>, u32>,
}

/// Immutable deduplicated string store backed by a shared byte slice.
#[derive(Clone)]
pub struct StringArena {
    pub(super) buf: std::sync::Arc<[u8]>,
    pub(super) index: HashMap<Box<str>, u32>,
}
