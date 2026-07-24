/// One ordered transition script for a table.
#[derive(Clone, Debug)]
pub struct TransitionEntry {
    /// Ordinal string, for example `"001"`.
    pub ordinal: String,
    /// 1-based identifier of the associated script.
    pub script_id: u32,
}

impl TransitionEntry {
    /// Creates a transition entry.
    pub fn new(ordinal: String, script_id: u32) -> Self {
        Self { ordinal, script_id }
    }
}
