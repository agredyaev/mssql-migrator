use crate::audit::HistoryRecord;

#[derive(Debug, Default)]
pub struct ApplyResult {
    pub applied: usize,
    pub skipped: usize,
    pub failed: usize,
    pub errors: Vec<String>,
    pub history: Vec<HistoryRecord>,
}

impl ApplyResult {
    pub fn push_error(&mut self, msg: String) {
        self.failed += 1;
        self.errors.push(msg);
    }
}
