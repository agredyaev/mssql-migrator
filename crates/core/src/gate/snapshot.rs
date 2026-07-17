use std::collections::HashMap;

use serde::{Deserialize, Serialize};

use crate::export::{MigrationPlan, PlannedObject};

/// Schema version tag for the snapshot format.
/// v2: object keys are database-qualified (`database/normalized_key`) so
/// same-named objects in different catalog databases cannot collapse.
pub const SNAPSHOT_VERSION: &str = "2";

/// Snapshot of the current migration plan, serialised between CLI runs.
#[derive(Debug, Default, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(default)]
pub struct PlanSnapshot {
    /// Map from normalized object key to its snapshot entry.
    pub objects: HashMap<String, SnapshotObject>,
    /// Format version string; compared against `SNAPSHOT_VERSION` on load.
    pub version: String,
    /// Hash of the script-layout on disk at snapshot time; empty when unused.
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub layout_hash: String,
    /// Whether the plan was blocked when this snapshot was taken.
    pub blocked: bool,
}

/// Per-object data captured in a `PlanSnapshot`.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct SnapshotObject {
    /// Filesystem path of the SQL script for this object.
    pub object_path: String,
    /// Planned action serialised as a string (e.g. `"CreateObject"`).
    pub planned_action: String,
    /// Hex-encoded SHA-256 checksum of the script at snapshot time.
    pub checksum_hex: String,
    /// Whether the object existed in the database at snapshot time.
    pub exists: bool,
}

impl PlanSnapshot {
    /// Builds a `PlanSnapshot` from a completed `MigrationPlan`.
    pub fn from_plan(plan: &MigrationPlan) -> Self {
        let mut objects = HashMap::new();
        for obj in &plan.objects {
            objects.insert(snapshot_key(obj), snapshot_object(obj));
        }
        Self {
            version: SNAPSHOT_VERSION.into(),
            blocked: plan.blocked,
            layout_hash: String::new(),
            objects,
        }
    }
}

/// Database-qualified identity: two catalog databases can hold the same
/// normalized key with different planned actions.
fn snapshot_key(obj: &PlannedObject) -> String {
    let db = obj.database_name.as_ref();
    if db.is_empty() {
        obj.normalized_key.as_ref().to_string()
    } else {
        format!("{db}/{}", obj.normalized_key.as_ref())
    }
}

fn snapshot_object(obj: &PlannedObject) -> SnapshotObject {
    SnapshotObject {
        object_path: obj.object_path.as_ref().to_string(),
        planned_action: serde_json::to_string(&obj.planned_action)
            .unwrap_or_default()
            .trim_matches('"')
            .to_string(),
        checksum_hex: hex::encode(obj.checksum),
        exists: obj.exists,
    }
}
