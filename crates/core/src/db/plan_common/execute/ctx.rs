use crate::config::Config;
use crate::db::plan_db_trace::PlanDbTrace;
use crate::domain::Workspace;

use super::super::types::{RunBodyContext, RunParallelContext};
use super::setup::ExecuteSetup;

pub(super) fn body_ctx<'a>(
    cfg: &'a Config,
    ws: &'a Workspace,
    keys_json: &'a str,
    setup: &'a ExecuteSetup,
    trace: &PlanDbTrace,
    bootstrap_in_sql: bool,
) -> RunBodyContext<'a> {
    RunBodyContext {
        cfg,
        ws,
        keys_json,
        db_fp: &setup.db_fp,
        git: &setup.git,
        full: setup.full,
        git_delta: setup.git_delta,
        need_checksums: setup.need_checksums,
        need_catalog: setup.need_catalog,
        catalog_base: setup.catalog_base.clone(),
        round_trips_start: trace.timings.round_trips,
        bootstrap_in_sql,
    }
}

pub(super) fn parallel_ctx<'a>(
    cfg: &'a Config,
    ws: &'a Workspace,
    keys_json: &'a str,
    setup: &'a ExecuteSetup,
) -> RunParallelContext<'a> {
    RunParallelContext {
        cfg,
        ws,
        keys_json,
        db_fp: &setup.db_fp,
        git: &setup.git,
        full: setup.full,
        git_delta: setup.git_delta,
        need_checksums: setup.need_checksums,
        need_catalog: setup.need_catalog,
        catalog_base: setup.catalog_base.clone(),
    }
}
