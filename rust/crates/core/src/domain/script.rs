use super::key::ScriptKey;
use super::shared::SharedStr;

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum ScriptKind {
    Object,
    Transition,
    Check,
}

#[derive(Clone, Debug)]
pub struct Script {
    pub key: ScriptKey,
    pub kind: ScriptKind,
    pub abs_path: SharedStr,
    pub schema: SharedStr,
    pub object_kind: SharedStr,
    pub object_name: SharedStr,
    pub checksum: Option<[u8; 32]>,
    pub git_hash: SharedStr,
    pub git_author: SharedStr,
    pub git_date: SharedStr,
    pub table_name: Option<String>,
    pub scaffold: bool,
}
