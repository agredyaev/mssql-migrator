//! Resolve the `plannedAt` stamp for the emitted plan.

/// Defaults to the current UTC time, but honors an override so CI can produce
/// byte-reproducible plan JSON: `RMIG_PLANNED_AT` (an explicit RFC3339 string)
/// takes precedence, then `SOURCE_DATE_EPOCH` (Unix seconds, the reproducible-build
/// convention). `plannedAt` is metadata only and is excluded from plan equivalence.
pub(super) fn resolved_planned_at() -> String {
    if let Ok(v) = std::env::var("RMIG_PLANNED_AT") {
        if !v.is_empty() {
            return v;
        }
    }
    if let Ok(epoch) = std::env::var("SOURCE_DATE_EPOCH") {
        if let Ok(secs) = epoch.trim().parse::<i64>() {
            if let Some(dt) = chrono::DateTime::from_timestamp(secs, 0) {
                return dt.to_rfc3339();
            }
        }
    }
    chrono::Utc::now().to_rfc3339()
}

#[cfg(test)]
mod tests {
    use super::resolved_planned_at;

    // Single test: it is the only place these env vars are touched, so there is
    // no cross-test race; keeping the steps in one test avoids racing itself.
    #[test]
    fn planned_at_override_precedence() {
        std::env::set_var("RMIG_PLANNED_AT", "2026-01-02T03:04:05+00:00");
        assert_eq!(resolved_planned_at(), "2026-01-02T03:04:05+00:00");

        std::env::remove_var("RMIG_PLANNED_AT");
        std::env::set_var("SOURCE_DATE_EPOCH", "0");
        assert!(
            resolved_planned_at().starts_with("1970-01-01T00:00:00"),
            "SOURCE_DATE_EPOCH should map to the unix epoch"
        );
        std::env::remove_var("SOURCE_DATE_EPOCH");
    }
}
