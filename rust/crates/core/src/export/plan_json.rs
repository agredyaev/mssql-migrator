use std::collections::HashMap;
use std::io::Write;

use serde::{Deserialize, Serialize};

use crate::domain::{Action, SchemaAction, SharedStr, StrOff, Workspace};
use crate::error::Result;

use super::plan_row::PlanRow;
use super::plan_side::PlanGitOff;

#[derive(Debug)]
pub struct MigrationPlan {
    pub command: String,
    pub planned_at: String,
    pub blocked: bool,
    pub blockers: Vec<String>,
    pub schemas: Vec<PlannedSchema>,
    /// Slim hot storage (**SLAB**). Filled by `compute_diff`; empty after `read_plan_json`.
    pub rows: Vec<PlanRow>,
    /// **SPARSE** git by row index (1:1 with layout row).
    pub plan_git: HashMap<u32, PlanGitOff>,
    /// **SPARSE** transition path arena offsets by row index (**VIEW** at materialize).
    pub plan_transitions: HashMap<u32, Vec<StrOff>>,
    /// Wire/cache; materialized on export/apply via [`super::materialize::materialize_planned_object`].
    pub objects: Vec<PlannedObject>,
    /// Reused prior digests by row index (filled in [`crate::plan::compute_diff_into`]).
    pub(crate) prior_by_row: Vec<Option<[u8; 32]>>,
    pub summary: PlanSummary,
}

impl Default for MigrationPlan {
    fn default() -> Self {
        Self {
            command: String::new(),
            planned_at: String::new(),
            blocked: false,
            blockers: Vec::new(),
            schemas: Vec::new(),
            rows: Vec::new(),
            plan_git: HashMap::new(),
            plan_transitions: HashMap::new(),
            objects: Vec::new(),
            prior_by_row: Vec::new(),
            summary: PlanSummary::default(),
        }
    }
}

/// JSON ingest shape (objects only on wire).
#[derive(Debug, Deserialize)]
struct MigrationPlanWire {
    #[serde(default)]
    command: String,
    #[serde(rename = "plannedAt", default)]
    planned_at: String,
    #[serde(default)]
    blocked: bool,
    #[serde(default)]
    blockers: Vec<String>,
    #[serde(default)]
    schemas: Vec<PlannedSchema>,
    objects: Vec<PlannedObject>,
    #[serde(default)]
    summary: PlanSummary,
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
    #[serde(rename = "plannedAction")]
    pub planned_action: Action,
    pub exists: bool,
    #[serde(with = "super::checksum_json")]
    pub checksum: [u8; 32],
    #[serde(flatten, skip_serializing_if = "Option::is_none")]
    pub git: Option<PlannedGit>,
    #[serde(
        rename = "transitionPaths",
        default,
        skip_serializing_if = "Vec::is_empty"
    )]
    pub transition_paths: Vec<SharedStr>,
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

pub fn write_plan_json(
    plan: &MigrationPlan,
    ws: Option<&Workspace>,
    w: &mut dyn Write,
) -> Result<()> {
    let v = if !plan.rows.is_empty() {
        let ws = ws.ok_or_else(|| {
            crate::error::Error::InvalidInput(
                "write_plan_json: workspace required for slim plan rows".into(),
            )
        })?;
        serde_json::to_string_pretty(&super::materialize::WireMigrationPlan::new(plan, ws))
    } else {
        serde_json::to_string_pretty(&super::materialize::PlanJsonFromObjects(plan))
    }
    .map_err(|e| crate::error::Error::Other(e.into()))?;
    w.write_all(v.as_bytes()).map_err(crate::error::Error::Io)?;
    w.write_all(b"\n").map_err(crate::error::Error::Io)?;
    Ok(())
}

pub fn read_plan_json(s: &str) -> Result<MigrationPlan> {
    let wire: MigrationPlanWire =
        serde_json::from_str(s).map_err(|e| crate::error::Error::InvalidInput(e.to_string()))?;
    Ok(MigrationPlan {
        command: wire.command,
        planned_at: wire.planned_at,
        blocked: wire.blocked,
        blockers: wire.blockers,
        schemas: wire.schemas,
        rows: Vec::new(),
        plan_git: HashMap::new(),
        plan_transitions: HashMap::new(),
        prior_by_row: Vec::new(),
        objects: wire.objects,
        summary: wire.summary,
    })
}
