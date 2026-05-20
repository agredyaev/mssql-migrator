use std::io::Write;

use serde::{Deserialize, Serialize};

use crate::domain::{Action, SchemaAction, SharedStr};
use crate::error::Result;

#[derive(Debug, Default, Serialize, Deserialize)]
pub struct MigrationPlan {
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub command: String,
    #[serde(rename = "plannedAt", default)]
    pub planned_at: String,
    pub blocked: bool,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub blockers: Vec<String>,
    pub schemas: Vec<PlannedSchema>,
    pub objects: Vec<PlannedObject>,
    pub summary: PlanSummary,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct PlannedSchema {
    #[serde(rename = "schemaName")]
    pub schema_name: String,
    pub action: SchemaAction,
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

#[derive(Debug, Serialize, Deserialize)]
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
    #[serde(rename = "databaseName", default, skip_serializing_if = "SharedStr::is_empty")]
    pub database_name: SharedStr,
    #[serde(rename = "parentName", default, skip_serializing_if = "SharedStr::is_empty")]
    pub parent_name: SharedStr,
    #[serde(rename = "plannedAction")]
    pub planned_action: Action,
    pub exists: bool,
    #[serde(with = "super::checksum_json")]
    pub checksum: [u8; 32],
    #[serde(flatten, skip_serializing_if = "Option::is_none")]
    pub git: Option<PlannedGit>,
    #[serde(rename = "transitionPaths", default, skip_serializing_if = "Vec::is_empty")]
    pub transition_paths: Vec<SharedStr>,
}

impl PlannedObject {
    pub fn git_hash(&self) -> &str {
        self.git
            .as_ref()
            .map(|g| g.hash.as_ref())
            .unwrap_or("")
    }

    pub fn git_author(&self) -> &str {
        self.git
            .as_ref()
            .map(|g| g.author.as_ref())
            .unwrap_or("")
    }

    pub fn git_date(&self) -> &str {
        self.git
            .as_ref()
            .map(|g| g.date.as_ref())
            .unwrap_or("")
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

pub fn write_plan_json(plan: &MigrationPlan, w: &mut dyn Write) -> Result<()> {
    let v = serde_json::to_string_pretty(plan).map_err(|e| crate::error::Error::Other(e.into()))?;
    w.write_all(v.as_bytes()).map_err(crate::error::Error::Io)?;
    w.write_all(b"\n").map_err(crate::error::Error::Io)?;
    Ok(())
}

pub fn read_plan_json(s: &str) -> Result<MigrationPlan> {
    serde_json::from_str(s).map_err(|e| crate::error::Error::InvalidInput(e.to_string()))
}
