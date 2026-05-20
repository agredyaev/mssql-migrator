use std::collections::HashMap;

use serde::{Deserialize, Serialize};

use crate::export::{MigrationPlan, PlannedObject};

pub const SNAPSHOT_VERSION: &str = "1";

#[derive(Debug, Default, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(default)]
pub struct PlanSnapshot {
    pub version: String,
    pub blocked: bool,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub layout_hash: String,
    pub objects: HashMap<String, SnapshotObject>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct SnapshotObject {
    pub object_path: String,
    pub planned_action: String,
    pub checksum_hex: String,
    pub exists: bool,
}

impl PlanSnapshot {
    pub fn from_plan(plan: &MigrationPlan) -> Self {
        let mut objects = HashMap::new();
        for obj in &plan.objects {
            objects.insert(obj.normalized_key.as_ref().to_string(), snapshot_object(obj));
        }
        Self {
            version: SNAPSHOT_VERSION.into(),
            blocked: plan.blocked,
            layout_hash: String::new(),
            objects,
        }
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
