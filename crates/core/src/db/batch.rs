//! Batch catalog operations: bootstrap SQL generation for plan-phase DB setup.

use crate::db::catalog::build_catalog_sql_batch;
use crate::sql;

/// Combined TDS batch: bootstrap, relaxed cache load (@p4 object count), catalog
/// (@p2 scope, @p3 schemas).
pub fn plan_db_batch_sql(
    kinds: &[&str],
    bootstrap: bool,
    catalog: bool,
    skip_schema_rows: bool,
    relaxed_cache: bool,
) -> String {
    let mut b = String::with_capacity(16_384);
    if bootstrap {
        b.push_str(sql::audit::BOOTSTRAP_TABLES);
        b.push('\n');
        b.push_str(sql::audit::BOOTSTRAP_DRIFT);
        b.push('\n');
    }
    if relaxed_cache {
        b.push_str(sql::catalog::CACHE_LOAD_RELAXED);
        b.push('\n');
    }
    if catalog {
        b.push_str(&build_catalog_sql_batch(kinds, skip_schema_rows));
    }
    b
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn batch_includes_catalog_params() {
        let sql = plan_db_batch_sql(&["tables"], true, true, false, false);
        assert!(sql.contains("OPENJSON(@p2)"), "catalog scope uses @p2");
        assert!(sql.contains("OPENJSON(@p3)"), "catalog schemas use @p3");
    }

    #[test]
    fn relaxed_cache_load_binds_count_as_param() {
        let sql = plan_db_batch_sql(&[], false, false, false, true);
        assert!(sql.contains("m.object_count = @p4"), "count bound as @p4");
    }
}
