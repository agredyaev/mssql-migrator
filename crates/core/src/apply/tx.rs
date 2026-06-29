use crate::sql;

pub fn wrap_transaction(body: &str) -> String {
    let mut out = String::with_capacity(
        sql::apply::BEGIN_TX.len() + body.len() + sql::apply::COMMIT_TX.len() + 2,
    );
    out.push_str(sql::apply::BEGIN_TX);
    out.push('\n');
    out.push_str(body);
    out.push('\n');
    out.push_str(sql::apply::COMMIT_TX);
    out
}

#[cfg(test)]
mod tests {
    use super::wrap_transaction;

    #[test]
    fn wrap_transaction_sets_xact_abort_before_begin() {
        let out = wrap_transaction("CREATE TABLE smoke.t1(id INT NOT NULL);");
        // XACT_ABORT ON must come first so a runtime error aborts the whole
        // transaction instead of leaving a partial commit.
        assert!(
            out.starts_with("SET XACT_ABORT ON"),
            "missing XACT_ABORT: {out}"
        );
        assert!(out.contains("BEGIN TRANSACTION"), "missing BEGIN: {out}");
        assert!(out.contains("CREATE TABLE smoke.t1"), "missing body: {out}");
        assert!(
            out.trim_end().ends_with("COMMIT TRANSACTION"),
            "missing COMMIT: {out}"
        );
        // XACT_ABORT precedes BEGIN precedes COMMIT.
        let xact = out.find("XACT_ABORT").expect("xact");
        let begin = out.find("BEGIN TRANSACTION").expect("begin");
        let commit = out.find("COMMIT TRANSACTION").expect("commit");
        assert!(xact < begin && begin < commit, "ordering wrong: {out}");
    }
}
