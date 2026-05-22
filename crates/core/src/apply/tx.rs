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
