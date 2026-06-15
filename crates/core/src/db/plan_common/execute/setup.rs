use std::time::Instant;

use crate::audit;
use crate::config::Config;
use crate::db::plan_db_trace::{PlanDbPath, PlanDbTrace};
use crate::db::state::CatalogState;
use crate::domain::Workspace;
use crate::driver::TimingConn;
use crate::error::Result;
use crate::gate::{resolve_changed_paths, ChangedPathsResult};
use crate::timings;

pub(super) struct ExecuteSetup {
    pub db_fp: String,
    pub git: ChangedPathsResult,
    pub full: bool,
    pub git_delta: bool,
    pub need_bootstrap: bool,
    pub need_checksums: bool,
    pub need_catalog: bool,
    pub defer_bootstrap: bool,
    pub bootstrap_in_sql: bool,
    pub catalog_base: Option<CatalogState>,
}

pub(super) async fn prepare_execute(
    cfg: &Config,
    conn: &mut TimingConn,
    ws: &Workspace,
    keys_json: &str,
    trace: &mut PlanDbTrace,
) -> Result<ExecuteSetup> {
    let db_fp = audit::db_fingerprint(&cfg.server, &cfg.database);
    audit::sync_tables_ensured(conn, &db_fp).await?;
    let git = resolve_changed_paths(&cfg.sql_root);
    let full = cfg.inspect_full() || cfg.skip_git() || git.full_inspect;
    let git_delta = !full && !git.paths.is_empty();
    let need_bootstrap = !audit::tables_ensured(&db_fp);
    trace.flags.bootstrap = need_bootstrap;
    let need_checksums = keys_json != "[]";

    let clean_git_tree =
        git.paths.is_empty() && matches!(git.source, "git-head" | "git-merge-base");
    let try_cache = cfg.catalog_cache()
        && audit::tables_ensured(&db_fp)
        && git.paths.is_empty()
        && !full
        && !clean_git_tree;

    trace.path = Some(if full {
        PlanDbPath::ColdFull
    } else if git_delta {
        PlanDbPath::GitDelta
    } else {
        PlanDbPath::Incremental
    });

    let t_cache = Instant::now();
    let catalog_base: Option<CatalogState> = if try_cache {
        crate::db::catalog_cache::try_load(conn, &ws.layout_digest, ws.object_count()).await?
    } else {
        None
    };
    trace.timings.cache_load_ms = timings::dur_ms(t_cache.elapsed());
    if catalog_base.is_some() {
        trace.timings.round_trips += 1;
    }

    let need_catalog = catalog_base.is_none();
    let defer_bootstrap = need_bootstrap && need_catalog && full && !git_delta;

    Ok(ExecuteSetup {
        db_fp,
        git,
        full,
        git_delta,
        need_bootstrap,
        need_checksums,
        need_catalog,
        defer_bootstrap,
        bootstrap_in_sql: defer_bootstrap,
        catalog_base,
    })
}
