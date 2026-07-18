use migrator_core::session::protocol::{Request, Response};

#[test]
fn request_ping_roundtrip_json() {
    let req = Request::Ping {
        server: "s".into(),
        port: "1433".into(),
        user: "sa".into(),
        encrypt: Some(true),
        trust_server_certificate: Some(false),
    };
    let line = serde_json::to_string(&req).unwrap();
    let back: Request = serde_json::from_str(&line).unwrap();
    assert!(matches!(back, Request::Ping { ref server, .. } if server == "s"));
}

/// A legacy client's bare `{"op":"ping"}` must still deserialize (endpoint
/// fields default to empty, which the daemon treats as "not declared").
#[test]
fn request_ping_without_endpoint_fields_is_accepted_regression() {
    let back: Request = serde_json::from_str(r#"{"op":"ping"}"#).unwrap();
    assert!(matches!(back, Request::Ping {
            ref server,
            ref port,
            ref user,
            encrypt: None,
            trust_server_certificate: None,
        } if server.is_empty() && port.is_empty() && user.is_empty()));
}

#[test]
fn request_auth_roundtrip_json() {
    let req = Request::Auth {
        token: "secret".into(),
    };
    let line = serde_json::to_string(&req).unwrap();
    let back: Request = serde_json::from_str(&line).unwrap();
    assert!(matches!(back, Request::Auth { .. }));
}

#[test]
fn response_err_json() {
    let resp = Response::err("boom");
    let line = serde_json::to_string(&resp).unwrap();
    let back: Response = serde_json::from_str(&line).unwrap();
    assert!(!back.ok);
    assert_eq!(back.error, "boom");
}
