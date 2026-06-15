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
        use serde::ser::SerializeStruct;
        let mut st = serializer.serialize_struct("PhaseTimings", 22)?;
        macro_rules! ser_i64 {
            ($f:ident) => {
                if self.$f != 0 {
                    st.serialize_field(stringify!($f), &self.$f)?;
                }
            };
        }
        ser_i64!(connect_ms);
        ser_i64!(scan_ms);
        ser_i64!(inspect_ms);
        ser_i64!(checksums_ms);
        ser_i64!(ensure_ms);
        ser_i64!(parallel_wall_ms);
        ser_i64!(audit_ms);
        ser_i64!(diff_ms);
        ser_i64!(plan_wall_ms);
        ser_i64!(cli_wall_ms);
        ser_i64!(engine_ms);
        ser_i64!(apply_ms);
        ser_i64!(audit_flush_ms);
        ser_i64!(plan_db_query_calls);
        ser_i64!(plan_db_query_ms);
        ser_i64!(plan_db_checksums_batch_ms);
        ser_i64!(plan_db_catalog_ms);
        ser_i64!(plan_db_catalog_sql_ms);
        ser_i64!(plan_db_intern_catalog_ms);
        ser_i64!(plan_db_round_trips);
        if !self.plan_db_path.is_empty() {
            st.serialize_field("plan_db_path", &self.plan_db_path)?;
        }
        if self.l1_cache_hit() {
            st.serialize_field("l1_cache_hit", &true)?;
        }
        if self.plan_db_bootstrap() {
            st.serialize_field("plan_db_bootstrap", &true)?;
        }
        if self.plan_db_catalog_queried() {
            st.serialize_field("plan_db_catalog_queried", &true)?;
        }
        if self.plan_db_history_empty() {
            st.serialize_field("plan_db_history_empty", &true)?;
        }
        if self.plan_db_checksums_skipped() {
            st.serialize_field("plan_db_checksums_skipped", &true)?;
        }
        st.end()
    }
}
