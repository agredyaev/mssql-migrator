use serde::{Deserialize, Deserializer, Serialize, Serializer};

use super::phase::PhaseTimings;
use super::wire::PhaseTimingsWire;

/// Custom `Serialize`: zero-valued timing fields and `false` boolean flags
/// are omitted for compact JSON. Custom `Deserialize` reads through the wire
/// intermediate, then converts.
impl<'de> Deserialize<'de> for PhaseTimings {
    fn deserialize<D: Deserializer<'de>>(deserializer: D) -> Result<Self, D::Error> {
        Ok(PhaseTimingsWire::deserialize(deserializer)?.into_phase_timings())
    }
}

impl Serialize for PhaseTimings {
    fn serialize<S: Serializer>(&self, serializer: S) -> Result<S::Ok, S::Error> {
        PhaseTimingsSerialize::from(self).serialize(serializer)
    }
}

#[derive(Serialize)]
struct PhaseTimingsSerialize<'a> {
    #[serde(skip_serializing_if = "is_zero_i64")]
    connect_ms: i64,
    #[serde(skip_serializing_if = "is_zero_i64")]
    scan_ms: i64,
    #[serde(skip_serializing_if = "is_zero_i64")]
    inspect_ms: i64,
    #[serde(skip_serializing_if = "is_zero_i64")]
    checksums_ms: i64,
    #[serde(skip_serializing_if = "is_zero_i64")]
    ensure_ms: i64,
    #[serde(skip_serializing_if = "is_zero_i64")]
    parallel_wall_ms: i64,
    #[serde(skip_serializing_if = "is_zero_i64")]
    audit_ms: i64,
    #[serde(skip_serializing_if = "is_zero_i64")]
    diff_ms: i64,
    #[serde(skip_serializing_if = "is_zero_i64")]
    plan_wall_ms: i64,
    #[serde(skip_serializing_if = "is_zero_i64")]
    cli_wall_ms: i64,
    #[serde(skip_serializing_if = "is_zero_i64")]
    engine_ms: i64,
    #[serde(skip_serializing_if = "is_zero_i64")]
    apply_ms: i64,
    #[serde(skip_serializing_if = "is_zero_i64")]
    audit_flush_ms: i64,
    #[serde(skip_serializing_if = "is_zero_i64")]
    plan_db_query_calls: i64,
    #[serde(skip_serializing_if = "is_zero_i64")]
    plan_db_query_ms: i64,
    #[serde(skip_serializing_if = "is_zero_i64")]
    plan_db_checksums_batch_ms: i64,
    #[serde(skip_serializing_if = "is_zero_i64")]
    plan_db_catalog_ms: i64,
    #[serde(skip_serializing_if = "is_zero_i64")]
    plan_db_catalog_sql_ms: i64,
    #[serde(skip_serializing_if = "is_zero_i64")]
    plan_db_intern_catalog_ms: i64,
    #[serde(skip_serializing_if = "is_zero_i64")]
    plan_db_round_trips: i64,
    #[serde(skip_serializing_if = "is_empty_str")]
    plan_db_path: &'a str,
    #[serde(skip_serializing_if = "is_false")]
    l1_cache_hit: bool,
    #[serde(skip_serializing_if = "is_false")]
    plan_db_bootstrap: bool,
    #[serde(skip_serializing_if = "is_false")]
    plan_db_catalog_queried: bool,
    #[serde(skip_serializing_if = "is_false")]
    plan_db_history_empty: bool,
    #[serde(skip_serializing_if = "is_false")]
    plan_db_checksums_skipped: bool,
}

impl<'a> From<&'a PhaseTimings> for PhaseTimingsSerialize<'a> {
    fn from(t: &'a PhaseTimings) -> Self {
        Self {
            connect_ms: t.connect_ms,
            scan_ms: t.scan_ms,
            inspect_ms: t.inspect_ms,
            checksums_ms: t.checksums_ms,
            ensure_ms: t.ensure_ms,
            parallel_wall_ms: t.parallel_wall_ms,
            audit_ms: t.audit_ms,
            diff_ms: t.diff_ms,
            plan_wall_ms: t.plan_wall_ms,
            cli_wall_ms: t.cli_wall_ms,
            engine_ms: t.engine_ms,
            apply_ms: t.apply_ms,
            audit_flush_ms: t.audit_flush_ms,
            plan_db_query_calls: t.plan_db_query_calls,
            plan_db_query_ms: t.plan_db_query_ms,
            plan_db_checksums_batch_ms: t.plan_db_checksums_batch_ms,
            plan_db_catalog_ms: t.plan_db_catalog_ms,
            plan_db_catalog_sql_ms: t.plan_db_catalog_sql_ms,
            plan_db_intern_catalog_ms: t.plan_db_intern_catalog_ms,
            plan_db_round_trips: t.plan_db_round_trips,
            plan_db_path: t.plan_db_path.as_str(),
            l1_cache_hit: t.l1_cache_hit(),
            plan_db_bootstrap: t.plan_db_bootstrap(),
            plan_db_catalog_queried: t.plan_db_catalog_queried(),
            plan_db_history_empty: t.plan_db_history_empty(),
            plan_db_checksums_skipped: t.plan_db_checksums_skipped(),
        }
    }
}

fn is_zero_i64(value: &i64) -> bool {
    *value == 0
}

fn is_false(value: &bool) -> bool {
    !*value
}

fn is_empty_str(value: &&str) -> bool {
    value.is_empty()
}
