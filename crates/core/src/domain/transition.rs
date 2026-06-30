use super::str_off::StrOff;

/// One transition script row for a table (sparse by object row id).
#[derive(Clone, Debug)]
pub struct TransitionEntry {
    /// Arena offset of the ordinal string (e.g. `"001"`).
    pub ord_off: StrOff,
    /// 1-based identifier of the associated script row.
    pub script_id: u32,
    pub(crate) staging_ord: Option<super::SharedStr>,
}

impl TransitionEntry {
    /// Creates a staging entry with an in-memory ordinal, before arena interning.
    pub fn new_staging(ordinal: super::SharedStr, script_id: u32) -> Self {
        Self {
            ord_off: StrOff::EMPTY,
            script_id,
            staging_ord: Some(ordinal),
        }
    }
}
