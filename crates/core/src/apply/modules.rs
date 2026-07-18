use crate::domain::Workspace;
use crate::driver::TimingConn;
use crate::error::Result;
use crate::export::PlannedObject;

use super::objects_exec::exec_object;
use super::result::ApplyResult;

pub(super) async fn apply_modules(
    conn: &mut TimingConn,
    ws: &Workspace,
    mut pending: Vec<&PlannedObject>,
    result: &mut ApplyResult,
) -> Result<()> {
    // ponytail: O(n^2) retries, replace with dependency extraction only if module batches grow large.
    while !pending.is_empty() {
        let mut next = Vec::new();
        let mut progress = false;
        let mut final_errors = Vec::new();
        for obj in pending {
            let failed = result.failed;
            let errors = result.errors.len();
            exec_object(conn, ws, obj, result).await?;
            if result.failed == failed {
                progress = true;
                continue;
            }
            let errors: Vec<_> = result.errors.drain(errors..).collect();
            result.failed = failed;
            if errors.iter().any(|e| e.contains("rollback failed")) {
                for error in errors {
                    result.push_error(error);
                }
                return Ok(());
            }
            final_errors.push(
                errors
                    .into_iter()
                    .last()
                    .unwrap_or_else(|| format!("{}: module apply failed", obj.normalized_key)),
            );
            next.push(obj);
        }
        if !progress {
            for error in final_errors {
                result.push_error(error);
            }
            return Ok(());
        }
        pending = next;
    }
    Ok(())
}
