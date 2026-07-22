use super::*;

#[test]
fn snapshot_json_is_well_formed_and_reflects_warm_flag() {
    mark_started();
    record_request();
    record_reconnect();
    record_queue_wait();

    let warm = snapshot_json(true);
    assert!(warm.contains("\"warm_connection\":true"), "{warm}");
    assert!(warm.contains("\"requests\":"), "{warm}");
    assert!(warm.contains("\"reconnects\":"), "{warm}");
    assert!(warm.contains("\"queue_waits\":"), "{warm}");
    assert!(warm.contains("\"uptime_s\":"), "{warm}");
    // Parses as JSON and the counters are non-negative after recording.
    let v: serde_json::Value = serde_json::from_str(&warm).expect("valid json");
    assert!(v["requests"].as_u64().unwrap() >= 1);
    assert!(v["reconnects"].as_u64().unwrap() >= 1);
    assert!(v["queue_waits"].as_u64().unwrap() >= 1);

    assert!(snapshot_json(false).contains("\"warm_connection\":false"));
}
