//! One DB session, one baseline, sequential git workflow phases (state assertions).
//!
//! Run:
//!   RMIG_RUN_SQLSERVER_INTEGRATION=1 cargo test -p migrator-core --profile release-fast --test workflow_integration -- --nocapture --test-threads=1

#[path = "common/integration_enabled.rs"]
mod integration_enabled;

#[path = "common/workflow_config.rs"]
mod workflow_config;

#[path = "common/db_reset.rs"]
mod db_reset;

#[path = "common/db_reset_workflow.rs"]
mod db_reset_workflow;

#[path = "common/state_smoke.rs"]
mod state_smoke;

#[path = "common/state_smoke_conn.rs"]
mod state_smoke_conn;

#[path = "common/state_ddl.rs"]
mod state_ddl;

#[path = "common/engine_smoke.rs"]
mod engine_smoke;

#[path = "common/workflow_git.rs"]
mod workflow_git;

#[path = "common/catalog.rs"]
mod catalog;

#[path = "common/workflow_engine.rs"]
mod workflow_engine;

use std::time::Instant;

use migrator_core::domain::Action;
use migrator_core::error::EXIT_PLAN_BLOCKED;

/// Single reset + baseline; then DDL (2 commits), module update, negative SQL — shared warm DB.
#[tokio::test(flavor = "current_thread")]
async fn workflow_git_scenarios_single_session() {
    if !integration_enabled::enabled() {
        eprintln!("skip: RMIG_RUN_SQLSERVER_INTEGRATION not set");
        return;
    }

    let t_all = Instant::now();
    let cfg = workflow_config::workflow_config();
    let git = workflow_git::GitRestore::open().expect("git");
    let table_sql =
        catalog::catalog_sql_rel(&cfg.sql_root, "smoke/tables/smoke_table.sql").expect("catalog");
    let view_sql =
        catalog::catalog_sql_rel(&cfg.sql_root, "smoke/views/smoke_view.sql").expect("catalog");
    let migration_dir =
        catalog::catalog_sql_rel(&cfg.sql_root, "smoke/tables/_migrations/smoke_table")
            .expect("catalog");

    db_reset_workflow::prepare_test_database(cfg)
        .await
        .expect("reset db");

    let t0 = Instant::now();
    let base = engine_smoke::baseline_migrate(cfg).await.expect("baseline");
    workflow_engine::log_timings("1-baseline", &base.timings);
    workflow_engine::assert_plan_db_par_slo("1-baseline", &base.timings);
    assert_eq!(base.exit_code, 0);

    let mut conn = state_smoke_conn::open_conn(cfg).await.expect("connect");
    state_smoke::assert_smoke_objects_materialized(&mut conn)
        .await
        .expect("smoke objects");
    let audit = state_smoke_conn::count_audit_rows(&mut conn, "object", "applied")
        .await
        .expect("audit count");
    assert!(audit >= 6, "audit rows: {audit}");
    eprintln!("phase 1 baseline OK (+{}ms)", t0.elapsed().as_millis());

    let t0 = Instant::now();
    let table_body =
        std::fs::read_to_string(workflow_git::sql_path(&table_sql)).expect("read table");
    let with_col = table_body.replacen(
        "created_at DATETIME2 NOT NULL DEFAULT SYSUTCDATETIME()",
        "created_at DATETIME2 NOT NULL DEFAULT SYSUTCDATETIME(),\n    added_at DATETIME2 NULL",
        1,
    );
    git.write_and_commit(&table_sql, &with_col, "test: ddl add added_at")
        .expect("commit ddl");

    let blocked = workflow_engine::migrate(cfg)
        .await
        .expect("blocked migrate");
    workflow_engine::log_timings("2-ddl-blocked", &blocked.timings);
    workflow_engine::assert_plan_db_par_slo("2-ddl-blocked", &blocked.timings);
    assert_eq!(blocked.exit_code, EXIT_PLAN_BLOCKED);

    let scaffold =
        state_ddl::read_scaffold_sql(std::path::Path::new(&cfg.sql_root)).expect("scaffold file");
    assert!(scaffold.contains("added_at") && scaffold.contains("ALTER TABLE"));

    assert!(
        !state_ddl::table_column_exists(&mut conn, "smoke", "smoke_table", "added_at")
            .await
            .expect("col probe"),
        "column not in DB while blocked"
    );

    git.add_tree_and_commit(&migration_dir, "test: track transition migration")
        .expect("commit migration dir");

    let mig_before = state_smoke_conn::count_audit_rows(&mut conn, "migration", "applied")
        .await
        .expect("migration rows before ddl");
    let ddl_apply = workflow_engine::migrate(cfg).await.expect("ddl apply");
    workflow_engine::log_timings("2-ddl-apply", &ddl_apply.timings);
    workflow_engine::assert_plan_db_par_slo("2-ddl-apply", &ddl_apply.timings);
    assert_eq!(ddl_apply.exit_code, 0);
    assert!(
        state_ddl::table_column_exists(&mut conn, "smoke", "smoke_table", "added_at")
            .await
            .expect("col probe"),
        "added_at must exist after transition"
    );
    // Transition persistence is kind='migration': the phase must prove that
    // exact row appeared, not merely re-count unrelated object rows.
    let mig_after = state_smoke_conn::count_audit_rows(&mut conn, "migration", "applied")
        .await
        .expect("migration rows after ddl");
    assert_eq!(
        mig_after,
        mig_before + 1,
        "exactly one migration audit row for the applied transition"
    );
    let audit_after_ddl = state_smoke_conn::count_audit_rows(&mut conn, "object", "applied")
        .await
        .expect("audit after ddl");
    assert!(
        audit_after_ddl > audit,
        "the completed transition also advances the table object baseline \
         (before={audit} after={audit_after_ddl})"
    );
    eprintln!("phase 2 ddl OK (+{}ms)", t0.elapsed().as_millis());

    let t0 = Instant::now();
    let updated_view = "CREATE OR ALTER VIEW smoke.smoke_view\nAS\nSELECT\n    id,\n    value,\n    created_at,\n    added_at,\n    CAST(1 AS INT) AS workflow_flag\nFROM smoke.smoke_table;\n";
    git.write_and_commit(&view_sql, updated_view, "test: extend smoke_view")
        .expect("commit view");

    let (plan, pt) = engine_smoke::plan(cfg).await.expect("view plan");
    workflow_engine::log_timings("3-view-plan", &pt);
    workflow_engine::assert_plan_db_par_slo("3-view-plan", &pt);
    let view_obj = plan
        .objects
        .iter()
        .find(|o| o.normalized_key.as_ref() == "smoke/views/smoke_view")
        .expect("view in plan");
    assert_eq!(view_obj.planned_action, Action::UpdateExistingModule);
    assert!(view_obj.exists);

    let view_apply = workflow_engine::migrate(cfg).await.expect("view migrate");
    workflow_engine::log_timings("3-view-apply", &view_apply.timings);
    workflow_engine::assert_plan_db_par_slo("3-view-apply", &view_apply.timings);
    assert_eq!(view_apply.exit_code, 0);

    let rows = conn
        .query(
            "SELECT COUNT(*) FROM sys.columns c
             INNER JOIN sys.views v ON v.object_id = c.object_id
             INNER JOIN sys.schemas s ON s.schema_id = v.schema_id
             WHERE s.name = 'smoke' AND v.name = 'smoke_view' AND c.name = 'workflow_flag'",
            &[],
        )
        .await
        .expect("view column");
    assert!(rows.first().and_then(|r| r.get_i32(0)).unwrap_or(0) > 0);
    let audit_after_view = state_smoke_conn::count_audit_rows(&mut conn, "object", "applied")
        .await
        .expect("audit after view");
    assert!(
        audit_after_view > audit_after_ddl,
        "audit must grow after view update (ddl={audit_after_ddl} view={audit_after_view})"
    );
    eprintln!("phase 3 view OK (+{}ms)", t0.elapsed().as_millis());

    let t0 = Instant::now();
    git.write_and_commit(
        &view_sql,
        "CREATE OR ALTER VIEW smoke.smoke_view\nAS\nSELEC 1 AS x;\n",
        "test: broken view",
    )
    .expect("commit broken view");

    match workflow_engine::migrate(cfg).await {
        Ok(out) => assert_ne!(out.exit_code, 0, "broken view must not succeed"),
        Err(migrator_core::error::Error::Sql(msg)) => {
            assert!(
                msg.contains("SELEC") || msg.to_lowercase().contains("syntax"),
                "{msg}"
            );
        }
        Err(e) => panic!("unexpected error: {e}"),
    }

    let rows = conn
        .query(
            "SELECT COUNT(*) FROM sys.columns c
             INNER JOIN sys.views v ON v.object_id = c.object_id
             INNER JOIN sys.schemas s ON s.schema_id = v.schema_id
             WHERE s.name = 'smoke' AND v.name = 'smoke_view' AND c.name = 'workflow_flag'",
            &[],
        )
        .await
        .expect("view preserved");
    assert!(
        rows.first().and_then(|r| r.get_i32(0)).unwrap_or(0) > 0,
        "working view must remain after failed migrate"
    );
    eprintln!("phase 4 negative OK (+{}ms)", t0.elapsed().as_millis());

    eprintln!(
        "workflow_git_scenarios_single_session TOTAL {}ms",
        t_all.elapsed().as_millis()
    );
}
