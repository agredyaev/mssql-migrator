use super::action::Action;
use super::key::{ObjectKey, ScriptKey};
use super::shared::SharedStr;

#[derive(Clone, Debug, Default)]
pub struct DbFacts {
    pub exists: bool,
    pub parent: Option<ObjectKey>,
}

#[derive(Clone, Debug)]
pub struct PlanDecision {
    pub action: Action,
    pub transition_paths: Vec<SharedStr>,
}

#[derive(Clone, Debug)]
pub struct ObjectEntry {
    pub key: ObjectKey,
    pub script: ScriptKey,
    pub history: Option<[u8; 32]>,
    pub db: DbFacts,
    pub plan: Option<PlanDecision>,
    pub checksum: [u8; 32],
    pub schema: SharedStr,
    pub kind: SharedStr,
    pub name: SharedStr,
    pub database_name: SharedStr,
    pub parent_name: SharedStr,
    pub parent_key: Option<ObjectKey>,
}
