//! Full SQL-side e2e invariants (no manual sqlcmd).

use migrator_core::config::Config;
use migrator_core::driver::TimingConn;
use migrator_core::error::{Error, Result};

#[path = "state_ddl.rs"]
mod state_ddl;

#[path = "state_smoke.rs"]
mod state_smoke;

use super::migrate::{
    assert_catalog_cache_when_enabled, assert_migration_history, AuditDbSnapshot,
};

const MIN_SMOKE_OBJECTS: i32 = 6;

pub async fn verify_cold_apply(
    cfg: &Config,
    conn: &mut TimingConn,
    snap: &AuditDbSnapshot,
) -> Result<()> {
    state_smoke::assert_smoke_objects_materialized(conn).await?;
    if snap.audit_object_rows < MIN_SMOKE_OBJECTS {
        return Err(Error::Other(anyhow::anyhow!(
            "audit object rows: want >={MIN_SMOKE_OBJECTS}, got {}",
            snap.audit_object_rows
        )));
    }
    if snap.audit_migration_rows != 0 {
        return Err(Error::Other(anyhow::anyhow!(
            "cold apply must not write migration history, got {}",
            snap.audit_migration_rows
        )));
    }
    assert_catalog_cache_when_enabled(cfg, snap)?;
    if cfg.catalog_cache() {
        verify_catalog_index_parents(conn).await?;
    }
    if cfg.catalog_cache() {
        let meta_count = catalog_meta_object_count(conn).await?;
        if meta_count != snap.audit_object_rows {
            return Err(Error::Other(anyhow::anyhow!(
                "catalog_meta.object_count={meta_count} != audit object rows {}",
                snap.audit_object_rows
            )));
        }
        if snap.catalog_cache_rows != snap.audit_object_rows {
            return Err(Error::Other(anyhow::anyhow!(
                "catalog_cache rows {} != audit object rows {}",
                snap.catalog_cache_rows,
                snap.audit_object_rows
            )));
        }
    }
    Ok(())
}

pub async fn verify_blocked_after_migrate(
    conn: &mut TimingConn,
    sql_root: &std::path::Path,
) -> Result<()> {
    let snap = super::migrate::snapshot_audit_db(conn).await?;
    if snap.audit_migration_rows != 0 {
        return Err(Error::Other(anyhow::anyhow!(
            "blocked migrate must not apply transitions (migration rows={})",
            snap.audit_migration_rows
        )));
    }
    if state_ddl::table_column_exists(conn, "smoke", "smoke_table", "added_at").await? {
        return Err(Error::Other(anyhow::anyhow!(
            "added_at must not exist in DB while plan is blocked"
        )));
    }
    let scaffold = state_ddl::read_scaffold_sql(sql_root).ok_or_else(|| {
        Error::Other(anyhow::anyhow!(
            "scaffold sql missing after blocked migrate"
        ))
    })?;
    if !(scaffold.contains("added_at") && scaffold.contains("ALTER TABLE")) {
        return Err(Error::Other(anyhow::anyhow!(
            "scaffold must contain added_at DDL, got: {}",
            &scaffold[..scaffold.len().min(120)]
        )));
    }
    Ok(())
}

pub async fn verify_ddl_transition_applied(
    cfg: &Config,
    conn: &mut TimingConn,
    sql_root: &std::path::Path,
) -> Result<AuditDbSnapshot> {
    let snap = super::migrate::snapshot_audit_db(conn).await?;
    let keys = super::migrate::migration_history_keys(conn).await?;
    assert_migration_history(&snap, &keys)?;
    assert_catalog_cache_when_enabled(cfg, &snap)?;
    verify_migration_row_content(conn, &keys).await?;

    if !state_ddl::table_column_exists(conn, "smoke", "smoke_table", "added_at").await? {
        return Err(Error::Other(anyhow::anyhow!(
            "added_at column missing after transition migrate"
        )));
    }
    if snap.audit_object_rows < MIN_SMOKE_OBJECTS {
        return Err(Error::Other(anyhow::anyhow!(
            "audit object rows after transition: {}",
            snap.audit_object_rows
        )));
    }
    if cfg.catalog_cache() {
        verify_catalog_index_parents(conn).await?;
        // Object history is an event log: a transition apply appends an extra
        // baseline record for its parent table, so compare distinct keys.
        let distinct_objects = count_distinct_object_keys(conn).await?;
        let meta_count = catalog_meta_object_count(conn).await?;
        if meta_count != distinct_objects {
            return Err(Error::Other(anyhow::anyhow!(
                "catalog_meta.object_count={meta_count} != distinct audit object keys {distinct_objects}"
            )));
        }
        if snap.catalog_cache_rows != distinct_objects {
            return Err(Error::Other(anyhow::anyhow!(
                "catalog_cache rows {} != distinct audit object keys {distinct_objects}",
                snap.catalog_cache_rows
            )));
        }
    }
    let _ = state_ddl::read_scaffold_sql(sql_root);
    Ok(snap)
}

async fn verify_catalog_index_parents(conn: &mut TimingConn) -> Result<()> {
    let rows = conn
        .query(
            "SELECT normalized_key, object_name, parent_name \
             FROM azdo_deploy_meta.catalog_cache \
             WHERE kind = 'indexes'",
            &[],
        )
        .await?;
    for row in rows {
        let key = row.get_str(0).unwrap_or("");
        let name = row.get_str(1).unwrap_or("");
        let parent = row.get_str(2).unwrap_or("");
        if parent.is_empty() {
            return Err(Error::Other(anyhow::anyhow!(
                "catalog_cache index {key} ({name}) missing parent_name"
            )));
        }
    }
    Ok(())
}

async fn verify_migration_row_content(conn: &mut TimingConn, keys: &[String]) -> Result<()> {
    let applied = super::migrate::count_audit_rows(conn, "migration").await?;
    if applied != i32::try_from(keys.len()).unwrap_or(i32::MAX) {
        return Err(Error::Other(anyhow::anyhow!(
            "migration history: count(kind=migration)={applied} != applied keys={:?}",
            keys
        )));
    }
    for key in keys {
        if !key.contains("_migrations/") || !key.ends_with(".sql") {
            return Err(Error::Other(anyhow::anyhow!(
                "migration history key must be _migrations/*.sql, got: {key}"
            )));
        }
    }
    Ok(())
}

async fn count_distinct_object_keys(conn: &mut TimingConn) -> Result<i32> {
    let rows = conn
        .query(
            "SELECT COUNT(DISTINCT normalized_key) FROM azdo_deploy_meta.history \
             WHERE kind = 'object'",
            &[],
        )
        .await?;
    Ok(rows.first().and_then(|r| r.get_i32(0)).unwrap_or(0))
}

async fn catalog_meta_object_count(conn: &mut TimingConn) -> Result<i32> {
    let rows = conn
        .query(
            "SELECT object_count FROM azdo_deploy_meta.catalog_meta WHERE id = 1",
            &[],
        )
        .await?;
    Ok(rows.first().and_then(|r| r.get_i32(0)).unwrap_or(0))
}
