//! JSON wire format types for plan output and ingest.
//!
//! ### Purpose
//! Defines the serialisation shape of `{command, plannedAt, schemas, objects,
//! summary}` emitted by `rmig plan --json` and re-ingested by `rmig migrate`
//! and gate comparison.

use serde::{Deserialize, Serialize};

use crate::domain::{Action, SchemaAction};

/// Top-level plan document: command metadata, schema/object lists, and summary counts.
#[derive(Debug, Default)]
pub struct MigrationPlan {
    /// Subcommand that generated the plan (`plan`, `migrate`, …).
    pub command: String,
    /// ISO-8601 timestamp when the plan was produced.
    pub planned_at: String,
    /// Human-readable blocker descriptions if the plan is blocked.
    pub blockers: Vec<String>,
    /// Schemas detected or created during planning.
    pub schemas: Vec<PlannedSchema>,
    /// Planned objects.
    pub objects: Vec<PlannedObject>,
    /// Aggregated counters (create, adopt, skip, …).
    pub summary: PlanSummary,
    /// True when blockers prevent execution.
    pub blocked: bool,
}

/// Schema entry in a plan: action (create / exists) and its SQL-qualified name.
#[derive(Debug, Serialize, Deserialize)]
pub struct PlannedSchema {
    /// Whether this schema needs creation or already exists.
    pub action: SchemaAction,
    /// SQL schema name (lowercased).
    #[serde(rename = "schemaName")]
    pub schema_name: String,
}

/// Git provenance for a planned object (commit hash, author, date).
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
pub struct PlannedGit {
    /// Full commit SHA.
    #[serde(rename = "gitHash")]
    pub hash: String,
    /// Author `<name> <<email>>` string.
    #[serde(rename = "gitAuthor")]
    pub author: String,
    /// ISO-8601 commit date.
    #[serde(rename = "gitDate")]
    pub date: String,
}

/// Single planned database object with its action, path, git metadata, and checksum.
#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct PlannedObject {
    /// Normalised key `schema/kind/name` (lowercased).
    #[serde(rename = "normalizedKey")]
    pub normalized_key: String,
    /// Relative SQL file path.
    #[serde(rename = "objectPath")]
    pub object_path: String,
    /// SQL schema name (lowercased).
    #[serde(rename = "schemaName")]
    pub schema_name: String,
    /// Object kind (`tables`, `views`, `procedures`, …).
    pub kind: String,
    /// SQL object name (lowercased).
    #[serde(rename = "objectName")]
    pub object_name: String,
    /// Database name (empty when single-database mode).
    #[serde(
        rename = "databaseName",
        default,
        skip_serializing_if = "String::is_empty"
    )]
    pub database_name: String,
    /// Parent object name (e.g. table for an index), empty for top-level objects.
    #[serde(
        rename = "parentName",
        default,
        skip_serializing_if = "String::is_empty"
    )]
    pub parent_name: String,
    /// Relative paths to transition scripts (`_migrations/`).
    #[serde(
        rename = "transitionPaths",
        default,
        skip_serializing_if = "Vec::is_empty"
    )]
    pub transition_paths: Vec<String>,
    /// Optional git provenance (hash, author, date).
    #[serde(flatten, skip_serializing_if = "Option::is_none")]
    pub git: Option<PlannedGit>,
    /// SHA-256 checksum of the object script.
    #[serde(with = "crate::export::checksum_json")]
    pub checksum: [u8; 32],
    /// What the plan intends to do with this object.
    #[serde(rename = "plannedAction")]
    pub planned_action: Action,
    /// True when the object exists in the database.
    pub exists: bool,
}

impl PlannedObject {
    /// Git commit SHA, or `""` when no git metadata is attached.
    pub fn git_hash(&self) -> &str {
        self.git.as_ref().map(|g| g.hash.as_ref()).unwrap_or("")
    }

    /// Git author string, or `""` when absent.
    pub fn git_author(&self) -> &str {
        self.git.as_ref().map(|g| g.author.as_ref()).unwrap_or("")
    }

    /// ISO-8601 git commit date, or `""` when absent.
    pub fn git_date(&self) -> &str {
        self.git.as_ref().map(|g| g.date.as_ref()).unwrap_or("")
    }
}

/// Aggregated counters summarising the plan at a glance.
#[derive(Debug, Default, Serialize, Deserialize)]
pub struct PlanSummary {
    /// Number of schemas in the plan.
    #[serde(rename = "schemaCount")]
    pub schema_count: usize,
    /// Total objects in the plan.
    #[serde(rename = "objectCount")]
    pub object_count: usize,
    /// Objects that will be created.
    #[serde(rename = "createCount")]
    pub create_count: usize,
    /// Objects adopted as-is (baseline).
    #[serde(rename = "adoptCount")]
    pub adopt_count: usize,
    /// Objects skipped (unchanged).
    #[serde(rename = "skipCount")]
    pub skip_count: usize,
    /// Objects with pending changes.
    #[serde(rename = "changedCount")]
    pub changed_count: usize,
    /// Objects blocked from processing.
    #[serde(rename = "blockedCount")]
    pub blocked_count: usize,
}
