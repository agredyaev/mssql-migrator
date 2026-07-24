use crate::export::{MigrationPlan, PlannedSchema};

pub(crate) fn prepare_plan_objects(plan: &mut MigrationPlan, n: usize) {
    plan.objects.truncate(n);
    plan.objects.reserve(n.saturating_sub(plan.objects.len()));
}

pub(crate) fn ensure_plan_schemas(plan: &mut MigrationPlan, n: usize) {
    plan.schemas.resize_with(n, || PlannedSchema {
        schema_name: String::new(),
        action: crate::domain::SchemaAction::Exists,
    });
}
