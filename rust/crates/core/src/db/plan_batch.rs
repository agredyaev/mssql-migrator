use std::time::Instant;

use crate::audit;
use crate::cache::l1::L1Cache;
use crate::config::Config;
use crate::db::batch;
use crate::db::catalog;
use crate::db::plan_db_trace::{PlanDbPath, PlanDbTrace};
use crate::db::state::{CatalogObject, CatalogState, ChecksumMap};
use crate::domain::{ObjectKey, Workspace};
use crate::driver::TimingConn;
use crate::error::Result;
use crate::gate::{expand_delta_closure, keys_for_changed_paths, resolve_changed_paths};
use crate::plan::git_hot_scope_json;
use crate::plan::scope::{build_inspect_scope, build_scope_json, InspectScope};
use crate::timings;

use super::plan_snapshot::PlanDbResult;

fn merge_stable_catalog(
    state: &mut CatalogState,
    stable: &std::collections::HashMap<ObjectKey, CatalogObject>,
) {
    for (k, o) in stable {
        state.objects.entry(k.clone()).or_insert_with(|| o.clone());
    }
}

pub async fn run_batch(
    cfg: &Config,
    conn: &mut TimingConn,
    ws: &Workspace,
    keys_json: &str,
    fp: &str,
    l1: &L1Cache,
) -> Result<PlanDbResult> {
    let db_fp = audit::db_fingerprint(&cfg.server, &cfg.database);
    let t0 = Instant::now();
    let mut trace = PlanDbTrace::default();

    let git = resolve_changed_paths(&cfg.sql_root);
    let full = cfg.inspect_full || cfg.skip_git || git.full_inspect;
    let git_delta = !full && !git.paths.is_empty();
    let need_bootstrap = !audit::tables_ensured(&db_fp);
    trace.bootstrap = need_bootstrap;
    let need_checksums = keys_json != "[]";

    let clean_git_tree = git.paths.is_empty()
        && matches!(git.source, "git-head" | "git-merge-base");
    let try_cache = cfg.catalog_cache
        && audit::tables_ensured(&db_fp)
        && git.paths.is_empty()
        && !full
        && !clean_git_tree;

    let t_cache = Instant::now();
    let mut catalog_base: Option<CatalogState> = if try_cache {
        crate::db::catalog_cache::try_load(conn, &ws.layout_digest, ws.object_count()).await?
    } else {
        None
    };
    trace.cache_load_ms = timings::dur_ms(t_cache.elapsed());

    let need_catalog = catalog_base.is_none();
    let mut checksums = ChecksumMap::new();
    let mut checksums_ms = 0i64;
    let mut inspect_ms = 0i64;

    trace.path = Some(if full {
        PlanDbPath::ColdFull
    } else if git_delta {
        PlanDbPath::GitDelta
    } else {
        PlanDbPath::Incremental
    });

    if git_delta {
        let t_git = Instant::now();
        let schemas_json = schemas_json(ws);
        let want_cache = cfg.catalog_cache && audit::tables_ensured(&db_fp);
        let mut loaded = CatalogState::default();
        let mut partial_cache = false;

        if need_checksums || need_bootstrap || want_cache {
            let sql = batch::plan_db_batch_sql(
                &[],
                need_bootstrap,
                need_checksums,
                false,
                false,
                false,
                if want_cache {
                    Some(ws.object_count())
                } else {
                    None
                },
            );
            if !sql.trim().is_empty() {
                let sets = conn
                    .query_all(&sql, &[keys_json, "[]", "[]"])
                    .await?;
                if need_bootstrap {
                    audit::mark_tables_ensured(&db_fp);
                }
                for set in sets {
                    if catalog::looks_like_cache_load_rows(&set) {
                        crate::db::catalog_cache::merge_load_rows(&mut loaded, &set)?;
                    } else if audit::looks_like_checksum_rows(&set) {
                        checksums = audit::checksum_map_from_rows(&set);
                        if !checksums.is_empty() {
                            audit::mark_history_nonempty(&db_fp);
                        }
                    }
                }
            }
        }
        if loaded.objects.len() == ws.object_count() {
            partial_cache = true;
            crate::db::intern_catalog_state(&mut loaded);
        } else {
            loaded = CatalogState::default();
        }
        checksums_ms = timings::dur_ms(t_git.elapsed());
        trace.checksums_batch_ms = checksums_ms;

        let scope = build_inspect_scope(ws, &git.paths, false, &checksums);
        let scope_json = build_scope_json(&scope);
        let query_catalog =
            should_query_catalog(false, &scope, &scope_json, &checksums).await?;
        trace.catalog_queried = query_catalog;

        let t_cat = Instant::now();
        let cache_covers_hot = partial_cache
            && scope.hot_keys.iter().all(|k| {
                loaded
                    .objects
                    .contains_key(&ObjectKey::from_normalized(k))
            });
        if query_catalog && !cache_covers_hot {
            if partial_cache {
                let kinds = kinds_for_scope(ws, &scope);
                let sql = batch::plan_db_batch_sql(
                    &kinds,
                    false,
                    false,
                    false,
                    true,
                    true,
                    None,
                );
                let sets = conn
                    .query_all(&sql, &["[]", &scope_json, &schemas_json])
                    .await?;
                for set in sets {
                    if catalog::looks_like_catalog_rows(&set) {
                        catalog::merge_rows(&mut loaded, &set)?;
                    }
                }
            } else {
                let hit_scope = git_hot_scope_json(ws, &git.paths);
                let kinds = kinds_for_git_delta(ws, &git.paths);
                let sql = batch::plan_db_batch_sql(
                    &kinds,
                    false,
                    false,
                    false,
                    true,
                    false,
                    None,
                );
                let sets = conn
                    .query_all(&sql, &["[]", &hit_scope, &schemas_json])
                    .await?;
                for set in sets {
                    if catalog::looks_like_catalog_rows(&set) {
                        catalog::merge_rows(&mut loaded, &set)?;
                    }
                }
            }
        }
        merge_stable_catalog(&mut loaded, &scope.stable_objects);
        crate::db::intern_catalog_state(&mut loaded);
        inspect_ms = timings::dur_ms(t_cat.elapsed());
        trace.catalog_ms = inspect_ms;
        catalog_base = Some(loaded);
    } else {
        if need_checksums || need_bootstrap {
            let t_cs = Instant::now();
            let sql = batch::plan_db_batch_sql(
                &[],
                need_bootstrap,
                need_checksums,
                false,
                false,
                false,
                None,
            );
            if !sql.trim().is_empty() {
                let sets = conn
                    .query_all(&sql, &[keys_json, "[]", "[]"])
                    .await?;
                if need_bootstrap {
                    audit::mark_tables_ensured(&db_fp);
                }
                for set in sets {
                    if audit::looks_like_checksum_rows(&set) {
                        checksums = audit::checksum_map_from_rows(&set);
                        if !checksums.is_empty() {
                            audit::mark_history_nonempty(&db_fp);
                        }
                    }
                }
            }
            checksums_ms = timings::dur_ms(t_cs.elapsed());
            trace.checksums_batch_ms = checksums_ms;
        }

        if need_catalog {
            let t_insp = Instant::now();
            let scope = build_inspect_scope(ws, &git.paths, full, &checksums);
            let scope_json = build_scope_json(&scope);
            let schemas_json = schemas_json(ws);

            let mut loaded = CatalogState::default();
            let query_catalog =
                should_query_catalog(full, &scope, &scope_json, &checksums).await?;
            trace.catalog_queried = query_catalog;
            if query_catalog {
                let kinds = kinds_for_scope(ws, &scope);
                let sql = batch::plan_db_batch_sql(&kinds, false, false, false, true, false, None);
                let sets = conn
                    .query_all(&sql, &["[]", &scope_json, &schemas_json])
                    .await?;
                for set in sets {
                    if catalog::looks_like_catalog_rows(&set) {
                        catalog::merge_rows(&mut loaded, &set)?;
                    }
                }
            }
            merge_stable_catalog(&mut loaded, &scope.stable_objects);
            crate::db::intern_catalog_state(&mut loaded);
            inspect_ms = timings::dur_ms(t_insp.elapsed());
            trace.catalog_ms = inspect_ms;
            catalog_base = Some(loaded);
        }
    }

    let catalog = catalog_base.unwrap_or_default();
    let parallel_wall = timings::dur_ms(t0.elapsed());

    let io = conn.io_snapshot();
    trace.query_calls = io.query_calls;
    trace.query_ms = io.query_ms;

    if cfg.catalog_cache && !catalog.objects.is_empty() && (need_catalog || git_delta) {
        let _ = crate::db::save_batched(conn, &ws.layout_digest, ws, &catalog).await;
    }

    l1.save(fp, &ws.layout_digest, &checksums, &catalog)?;

    Ok(PlanDbResult {
        checksums,
        catalog,
        ensure_ms: if need_bootstrap {
            checksums_ms / 2
        } else {
            0
        },
        checksums_ms,
        inspect_ms,
        parallel_wall_ms: parallel_wall,
        l1_hit: false,
        trace,
    })
}

fn schemas_json(ws: &Workspace) -> String {
    let schemas: Vec<String> = ws
        .schemas
        .iter()
        .map(|s| s.normalized.as_ref().to_string())
        .collect();
    serde_json::to_string(&schemas).unwrap_or_else(|_| "[]".into())
}

fn kinds_for_git_delta<'a>(ws: &'a Workspace, changed_paths: &[String]) -> Vec<&'a str> {
    let delta = expand_delta_closure(ws, keys_for_changed_paths(ws, changed_paths));
    let mut kinds = Vec::new();
    for o in &ws.object_entries {
        if delta.contains(o.key.as_str()) {
            kinds.push(o.kind.as_ref());
        }
    }
    kinds
}

fn kinds_for_scope<'a>(ws: &'a Workspace, scope: &InspectScope) -> Vec<&'a str> {
    let mut kinds = Vec::new();
    for o in &ws.object_entries {
        if scope.full_inspect || scope.hot_keys.contains(o.key.as_str()) {
            kinds.push(o.kind.as_ref());
        }
    }
    kinds
}

fn hot_keys_have_history(scope: &InspectScope, checksums: &ChecksumMap) -> bool {
    scope.hot_keys.iter().any(|k| {
        let key = ObjectKey::from_normalized(k);
        checksums
            .get(&key)
            .is_some_and(|cs| cs != &[0; 32])
    })
}

async fn should_query_catalog(
    full: bool,
    scope: &InspectScope,
    scope_json: &str,
    checksums: &ChecksumMap,
) -> Result<bool> {
    if scope_json == "[]" {
        return Ok(false);
    }
    if full && checksums.is_empty() {
        return Ok(false);
    }
    if hot_keys_have_history(scope, checksums) {
        return Ok(true);
    }
    if !full && !scope.hot_keys.is_empty() {
        return Ok(true);
    }
    Ok(false)
}
