use crate::config::Config;
use crate::db::plan_db_trace::PlanDbTrace;
use crate::db::state::{CatalogState, ChecksumMap};
use crate::domain::Workspace;
use crate::gate::ChangedPathsResult;

pub(super) struct RunBodyContext<'a> {
    pub cfg: &'a Config,
    pub ws: &'a Workspace,
    pub keys_json: &'a str,
    pub db_fp: &'a str,
    pub git: &'a ChangedPathsResult,
    pub full: bool,
    pub git_delta: bool,
    pub need_checksums: bool,
    pub need_catalog: bool,
    pub catalog_base: Option<CatalogState>,
    pub round_trips_start: i64,
    pub bootstrap_in_sql: bool,
    pub bypass: bool,
    pub allow_checksum_repair: bool,
}

pub(crate) struct BodyOutput {
    pub checksums: ChecksumMap,
    pub catalog: CatalogState,
    pub checksums_ms: i64,
    pub inspect_ms: i64,
    pub ensure_ms: i64,
    pub trace: PlanDbTrace,
    pub _round_trips: i64,
}
