use crate::domain::Action;
use crate::export::{MigrationPlan, PlanRow, PlannedSchema};

pub(crate) fn ensure_plan_rows(plan: &mut MigrationPlan, n: usize) {
    plan.rows.resize_with(n, empty_plan_row);
    plan.objects.clear();
}

pub(crate) fn ensure_plan_schemas(plan: &mut MigrationPlan, n: usize) {
    plan.schemas.resize_with(n, || PlannedSchema {
        schema_name: String::new(),
        action: crate::domain::SchemaAction::Exists,
    });
}

fn empty_plan_row() -> PlanRow {
    PlanRow {
        action: Action::SkipUnchanged.as_repr(),
        flags: 0,
        checksum: [0; 32],
    }
}
