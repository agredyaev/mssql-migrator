use crate::domain::Action;
use crate::export::{MigrationPlan, PlanRow, PlannedSchema};

pub(crate) fn ensure_plan_rows(plan: &mut MigrationPlan, n: usize) {
    if plan.rows.capacity() < n {
        plan.rows.reserve(n - plan.rows.capacity());
    }
    if plan.rows.len() < n {
        plan.rows.resize_with(n, empty_plan_row);
    } else {
        plan.rows.truncate(n);
    }
    plan.objects.clear();
}

pub(crate) fn ensure_plan_schemas(plan: &mut MigrationPlan, n: usize) {
    if plan.schemas.capacity() < n {
        plan.schemas.reserve(n - plan.schemas.capacity());
    }
    if plan.schemas.len() < n {
        plan.schemas.resize_with(n, || PlannedSchema {
            schema_name: String::new(),
            action: crate::domain::SchemaAction::Exists,
        });
    } else {
        plan.schemas.truncate(n);
    }
}

fn empty_plan_row() -> PlanRow {
    PlanRow {
        action: Action::SkipUnchanged.as_repr(),
        flags: 0,
        checksum: [0; 32],
    }
}
