use super::super::timing_compare::compare_workflow_timings;
use super::super::types::E2EApplyReport;

/// Compares two `E2EApplyReport` values and returns a list of mismatch descriptions.
pub fn compare_e2e_apply_reports(
    baseline: &E2EApplyReport,
    actual: &E2EApplyReport,
) -> Vec<String> {
    let mut msgs = Vec::new();
    if baseline.scenario != actual.scenario {
        msgs.push(format!(
            "scenario: baseline={} actual={}",
            baseline.scenario, actual.scenario
        ));
    }
    if baseline.applied != actual.applied {
        msgs.push(format!(
            "applied: baseline={} actual={}",
            baseline.applied, actual.applied
        ));
    }
    if baseline.failed != actual.failed {
        msgs.push(format!(
            "failed: baseline={} actual={}",
            baseline.failed, actual.failed
        ));
    }
    if baseline.skipped != actual.skipped {
        msgs.push(format!(
            "skipped: baseline={} actual={}",
            baseline.skipped, actual.skipped
        ));
    }
    if baseline.audit_object_rows != actual.audit_object_rows {
        msgs.push(format!(
            "audit_object_rows: baseline={} actual={}",
            baseline.audit_object_rows, actual.audit_object_rows
        ));
    }
    if baseline.audit_migration_rows != actual.audit_migration_rows {
        msgs.push(format!(
            "audit_migration_rows: baseline={} actual={}",
            baseline.audit_migration_rows, actual.audit_migration_rows
        ));
    }
    if baseline.catalog_meta_rows != actual.catalog_meta_rows {
        msgs.push(format!(
            "catalog_meta_rows: baseline={} actual={}",
            baseline.catalog_meta_rows, actual.catalog_meta_rows
        ));
    }
    if baseline.catalog_cache_rows != actual.catalog_cache_rows {
        msgs.push(format!(
            "catalog_cache_rows: baseline={} actual={}",
            baseline.catalog_cache_rows, actual.catalog_cache_rows
        ));
    }
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
