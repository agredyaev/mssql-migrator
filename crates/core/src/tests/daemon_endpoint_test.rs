use super::{endpoint_mismatch, Endpoint};

fn daemon() -> Endpoint {
    Endpoint {
        server: "server-a".into(),
        port: "1433".into(),
        user: "sa".into(),
    }
}

#[test]
fn matching_endpoint_is_accepted_happy_path() {
    assert!(endpoint_mismatch(&daemon(), "server-a", "1433", "sa").is_none());
}

#[test]
fn legacy_client_with_empty_fields_is_accepted_edge_case() {
    assert!(endpoint_mismatch(&daemon(), "", "", "").is_none());
}

#[test]
fn different_server_is_refused_regression() {
    let msg = endpoint_mismatch(&daemon(), "server-b", "1433", "sa").expect("must refuse");
    assert!(msg.contains("server"), "names the field: {msg}");
    assert!(msg.contains("server-a"), "names the daemon target: {msg}");
}

#[test]
fn different_port_or_user_is_refused_edge_case() {
    assert!(endpoint_mismatch(&daemon(), "server-a", "1434", "sa").is_some());
    assert!(endpoint_mismatch(&daemon(), "server-a", "1433", "deploy").is_some());
}
