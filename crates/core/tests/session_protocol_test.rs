use migrator_core::session::protocol::{Request, Response};

#[test]
fn request_ping_roundtrip_json() {
    let req = Request::Ping;
    let line = serde_json::to_string(&req).unwrap();
    let back: Request = serde_json::from_str(&line).unwrap();
    assert!(matches!(back, Request::Ping));
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
