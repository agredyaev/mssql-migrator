use crate::db::catalog;
use crate::sql;

/// Combined TDS batch: bootstrap, checksums (@p1), scoped hit (@p2), catalog (@p2 scope, @p3 schemas).
pub fn plan_db_batch_sql(
    kinds: &[&str],
    bootstrap: bool,
    checksums: bool,
    scoped_hit: bool,
    catalog: bool,
    skip_schema_rows: bool,
    relaxed_cache_count: Option<usize>,
) -> String {
    let mut b = String::with_capacity(16_384);
    if bootstrap {
        b.push_str(sql::audit::BOOTSTRAP_TABLES);
        b.push('\n');
        b.push_str(sql::audit::BOOTSTRAP_INDEX);
        b.push('\n');
    }
    if let Some(count) = relaxed_cache_count {
        b.push_str(&sql::catalog::CACHE_LOAD_RELAXED.replace("@p1", &count.to_string()));
        b.push('\n');
    }
    if checksums {
        b.push_str(sql::audit::LOAD_CHECKSUMS);
        b.push('\n');
    }
    if scoped_hit {
        b.push_str(&catalog::scoped_hit_sql_batch());
        b.push('\n');
    }
    if catalog {
        b.push_str(&catalog::build_catalog_sql_batch(kinds, skip_schema_rows));
    }
    b
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn batch_includes_checksums_and_catalog_params() {
        let sql = plan_db_batch_sql(&["tables"], true, true, false, true, false, None);
        assert!(sql.contains("OPENJSON(@p1)"), "checksums use @p1");
        assert!(sql.contains("OPENJSON(@p2)"), "catalog scope uses @p2");
        assert!(sql.contains("OPENJSON(@p3)"), "catalog schemas use @p3");
    }
}
