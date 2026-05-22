use serde::{Deserialize, Serialize};

use std::collections::HashMap;

use crate::domain::{Action, SchemaAction, SharedStr, StrOff};
use crate::export::plan_row::PlanRow;
use crate::export::plan_side::PlanGitOff;

#[derive(Debug, Default)]
pub struct MigrationPlan {
    pub command: String,
    pub planned_at: String,
    pub blockers: Vec<String>,
    pub schemas: Vec<PlannedSchema>,
    pub rows: Vec<PlanRow>,
    pub plan_git: HashMap<u32, PlanGitOff>,
    pub plan_transitions: HashMap<u32, Vec<StrOff>>,
    pub objects: Vec<PlannedObject>,
    pub summary: PlanSummary,
    pub blocked: bool,
}

/// JSON ingest shape (objects only on wire).
#[derive(Debug, Deserialize)]
pub(super) struct MigrationPlanWire {
    #[serde(default)]
    pub(super) command: String,
    #[serde(rename = "plannedAt", default)]
    pub(super) planned_at: String,
    #[serde(default)]
    pub(super) blocked: bool,
    #[serde(default)]
    pub(super) blockers: Vec<String>,
    #[serde(default)]
    pub(super) schemas: Vec<PlannedSchema>,
    pub(super) objects: Vec<PlannedObject>,
    #[serde(default)]
    pub(super) summary: PlanSummary,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct PlannedSchema {
    pub action: SchemaAction,
    #[serde(rename = "schemaName")]
    pub schema_name: String,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
pub struct PlannedGit {
    #[serde(rename = "gitHash")]
    pub hash: SharedStr,
    #[serde(rename = "gitAuthor")]
    pub author: SharedStr,
    #[serde(rename = "gitDate")]
    pub date: SharedStr,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct PlannedObject {
    #[serde(rename = "normalizedKey")]
    pub normalized_key: SharedStr,
    #[serde(rename = "objectPath")]
    pub object_path: SharedStr,
    #[serde(rename = "schemaName")]
    pub schema_name: SharedStr,
    pub kind: SharedStr,
    #[serde(rename = "objectName")]
    pub object_name: SharedStr,
    #[serde(
        rename = "databaseName",
        default,
        skip_serializing_if = "SharedStr::is_empty"
    )]
    pub database_name: SharedStr,
    #[serde(
        rename = "parentName",
        default,
        skip_serializing_if = "SharedStr::is_empty"
    )]
    pub parent_name: SharedStr,
    #[serde(
        rename = "transitionPaths",
        default,
        skip_serializing_if = "Vec::is_empty"
    )]
    pub transition_paths: Vec<SharedStr>,
    #[serde(flatten, skip_serializing_if = "Option::is_none")]
    pub git: Option<PlannedGit>,
    #[serde(with = "crate::export::checksum_json")]
    pub checksum: [u8; 32],
    #[serde(rename = "plannedAction")]
    pub planned_action: Action,
    pub exists: bool,
}

impl PlannedObject {
    pub fn git_hash(&self) -> &str {
        self.git.as_ref().map(|g| g.hash.as_ref()).unwrap_or("")
    }

    pub fn git_author(&self) -> &str {
        self.git.as_ref().map(|g| g.author.as_ref()).unwrap_or("")
    }

    pub fn git_date(&self) -> &str {
        self.git.as_ref().map(|g| g.date.as_ref()).unwrap_or("")
    }
}

#[derive(Debug, Default, Serialize, Deserialize)]
pub struct PlanSummary {
    #[serde(rename = "schemaCount")]
    pub schema_count: usize,
    #[serde(rename = "objectCount")]
    pub object_count: usize,
    #[serde(rename = "createCount")]
    pub create_count: usize,
    #[serde(rename = "adoptCount")]
    pub adopt_count: usize,
    #[serde(rename = "skipCount")]
    pub skip_count: usize,
    #[serde(rename = "changedCount")]
    pub changed_count: usize,
    #[serde(rename = "blockedCount")]
    pub blocked_count: usize,
}
