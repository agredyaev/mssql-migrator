//! Batch catalog operations: bootstrap SQL generation for plan-phase DB setup.

use crate::db::catalog;
use crate::sql;

/// Plan-path bootstrap: tables + drift trigger (index deferred to apply).
pub fn plan_bootstrap_tables_sql() -> String {
    format!(
        "{}\n{}\n",
        sql::audit::BOOTSTRAP_TABLES,
        sql::audit::BOOTSTRAP_DRIFT
    )
}

struct PlanDbBatchSqlOpts<'a> {
    kinds: &'a [&'a str],
    bootstrap: bool,
    catalog: bool,
    skip_schema_rows: bool,
    relaxed_cache: bool,
}

/// Combined TDS batch: bootstrap, relaxed cache load (@p4 object count), catalog
/// (@p2 scope, @p3 schemas).
pub fn plan_db_batch_sql(
    kinds: &[&str],
    bootstrap: bool,
    catalog: bool,
    skip_schema_rows: bool,
    relaxed_cache: bool,
) -> String {
    plan_db_batch_sql_inner(&PlanDbBatchSqlOpts {
        kinds,
        bootstrap,
        catalog,
        skip_schema_rows,
        relaxed_cache,
    })
}

fn plan_db_batch_sql_inner(opts: &PlanDbBatchSqlOpts<'_>) -> String {
    let mut b = String::with_capacity(16_384);
    if opts.bootstrap {
        b.push_str(sql::audit::BOOTSTRAP_TABLES);
        b.push('\n');
        b.push_str(sql::audit::BOOTSTRAP_DRIFT);
        b.push('\n');
    }
    if opts.relaxed_cache {
        b.push_str(sql::catalog::CACHE_LOAD_RELAXED);
        b.push('\n');
    }
    if opts.catalog {
        b.push_str(&catalog::build_catalog_sql_batch(
            opts.kinds,
            opts.skip_schema_rows,
        ));
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
