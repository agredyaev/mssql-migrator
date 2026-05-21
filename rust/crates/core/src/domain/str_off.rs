/// Offset+length into [`super::arena::LayoutArena`] (**ARENA** / **IDX**).
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq, Hash)]
pub struct StrOff(pub u32, pub u32);
