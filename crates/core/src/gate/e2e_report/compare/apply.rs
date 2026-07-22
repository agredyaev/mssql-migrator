use super::super::timing_compare::compare_workflow_timings;
use super::super::types::E2EApplyReport;
use super::diff_field;

/// Compares two `E2EApplyReport` values and returns a list of mismatch descriptions.
pub fn compare_e2e_apply_reports(
    baseline: &E2EApplyReport,
    actual: &E2EApplyReport,
) -> Vec<String> {
    let mut msgs = Vec::new();
    diff_field(&mut msgs, "scenario", &baseline.scenario, &actual.scenario);
    diff_field(&mut msgs, "applied", &baseline.applied, &actual.applied);
    diff_field(&mut msgs, "failed", &baseline.failed, &actual.failed);
    diff_field(&mut msgs, "skipped", &baseline.skipped, &actual.skipped);
    diff_field(
        &mut msgs,
        "audit_object_rows",
        &baseline.audit_object_rows,
        &actual.audit_object_rows,
    );
    diff_field(
        &mut msgs,
        "audit_migration_rows",
        &baseline.audit_migration_rows,
        &actual.audit_migration_rows,
    );
    diff_field(
        &mut msgs,
        "catalog_meta_rows",
        &baseline.catalog_meta_rows,
        &actual.catalog_meta_rows,
    );
    diff_field(
        &mut msgs,
        "catalog_cache_rows",
        &baseline.catalog_cache_rows,
        &actual.catalog_cache_rows,
    );
    if baseline.errors != actual.errors {
        msgs.push(format!(
            "errors: baseline={:?} actual={:?}",
            baseline.errors, actual.errors
        ));
    }
    if baseline.setup_steps != actual.setup_steps {
        msgs.push(format!(
            "setup_steps: baseline={:?} actual={:?}",
            baseline.setup_steps, actual.setup_steps
        ));
    }
    if baseline.timings.total_ms > 0 || actual.timings.total_ms > 0 {
        msgs.extend(compare_workflow_timings(&baseline.timings, &actual.timings));
    }
    msgs
}
