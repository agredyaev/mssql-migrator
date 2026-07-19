//! Batch catalog operations: bootstrap SQL generation for plan-phase DB setup.

use crate::db::catalog;
use crate::sql;

/// Plan-path bootstrap: tables only (index deferred to apply).
pub fn plan_bootstrap_tables_sql() -> String {
    format!("{}\n", sql::audit::BOOTSTRAP_TABLES)
}

struct PlanDbBatchSqlOpts<'a> {
    kinds: &'a [&'a str],
    bootstrap: bool,
    bootstrap_include_index: bool,
    checksums: bool,
    scoped_hit: bool,
    catalog: bool,
    skip_schema_rows: bool,
    relaxed_cache: bool,
}

/// Combined TDS batch: bootstrap, checksums (@p1), scoped hit (@p2), catalog
/// (@p2 scope, @p3 schemas), relaxed cache load (@p4 object count).
pub fn plan_db_batch_sql(
    kinds: &[&str],
    bootstrap: bool,
    checksums: bool,
    scoped_hit: bool,
    catalog: bool,
    skip_schema_rows: bool,
    relaxed_cache: bool,
) -> String {
    plan_db_batch_sql_inner(&PlanDbBatchSqlOpts {
        kinds,
        bootstrap,
        bootstrap_include_index: false,
        checksums,
        scoped_hit,
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
        if opts.bootstrap_include_index {
            b.push_str(sql::audit::BOOTSTRAP_INDEX);
            b.push('\n');
        }
    }
    if opts.relaxed_cache {
        b.push_str(sql::catalog::CACHE_LOAD_RELAXED);
        b.push('\n');
    }
    if opts.checksums {
        b.push_str(sql::audit::LOAD_CHECKSUMS);
        b.push('\n');
    }
    if opts.scoped_hit {
        b.push_str(&catalog::scoped_hit_sql_batch());
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
    fn batch_includes_checksums_and_catalog_params() {
        let sql = plan_db_batch_sql(&["tables"], true, true, false, true, false, false);
        assert!(sql.contains("OPENJSON(@p1)"), "checksums use @p1");
        assert!(sql.contains("OPENJSON(@p2)"), "catalog scope uses @p2");
        assert!(sql.contains("OPENJSON(@p3)"), "catalog schemas use @p3");
    }

    #[test]
    fn relaxed_cache_load_binds_count_as_param() {
        let sql = plan_db_batch_sql(&[], false, false, false, false, false, true);
        assert!(sql.contains("m.object_count = @p4"), "count bound as @p4");
    }
}
