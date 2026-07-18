use super::{endpoint_mismatch, Endpoint};

fn daemon() -> Endpoint {
    Endpoint {
        server: "server-a".into(),
        port: "1433".into(),
        user: "sa".into(),
        encrypt: true,
        trust_server_certificate: false,
    }
}

#[test]
fn matching_endpoint_is_accepted_happy_path() {
    assert!(
        endpoint_mismatch(&daemon(), "server-a", "1433", "sa", Some(true), Some(false)).is_none()
    );
}

#[test]
fn legacy_client_with_empty_fields_is_accepted_edge_case() {
    assert!(endpoint_mismatch(&daemon(), "", "", "", None, None).is_none());
}

#[test]
fn different_server_is_refused_regression() {
    let msg = endpoint_mismatch(&daemon(), "server-b", "1433", "sa", Some(true), Some(false))
        .expect("must refuse");
    assert!(msg.contains("server"), "names the field: {msg}");
    assert!(msg.contains("server-a"), "names the daemon target: {msg}");
}

#[test]
fn different_port_or_user_is_refused_edge_case() {
    assert!(endpoint_mismatch(&daemon(), "server-a", "1434", "sa", None, None).is_some());
    assert!(endpoint_mismatch(&daemon(), "server-a", "1433", "deploy", None, None).is_some());
}

#[test]
fn different_tls_policy_is_refused_regression() {
    let msg = endpoint_mismatch(&daemon(), "server-a", "1433", "sa", Some(false), Some(true))
        .expect("must refuse");
    assert!(msg.contains("encrypt"), "names encrypt mismatch: {msg}");
    assert!(
        msg.contains("trust_server_certificate"),
        "names certificate-policy mismatch: {msg}"
    );
}
