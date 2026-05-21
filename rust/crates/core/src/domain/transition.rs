use super::str_off::StrOff;

/// One transition script row for a table (**W3** / sparse by object row id).
#[derive(Clone, Debug)]
pub struct TransitionEntry {
    pub ord_off: StrOff,
    pub script_id: u32,
    pub(crate) staging_ord: Option<super::SharedStr>,
}

impl TransitionEntry {
    pub fn new_staging(ordinal: super::SharedStr, script_id: u32) -> Self {
        Self {
            ord_off: StrOff::EMPTY,
            script_id,
            staging_ord: Some(ordinal),
        }
    }
}
