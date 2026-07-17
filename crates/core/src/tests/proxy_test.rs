use tokio::io::{AsyncBufReadExt, AsyncWriteExt, BufReader};
use tokio::net::UnixStream;

use crate::session::limits::MAX_SESSION_LINE_BYTES;
use crate::session::protocol::{Request, Response};
use crate::session::proxy::ProxyClient;

fn ping_request() -> Request {
    Request::Ping {
        server: String::new(),
        port: String::new(),
        user: String::new(),
    }
}

fn proxy_client(stream: UnixStream) -> ProxyClient {
    let (read_half, write_half) = stream.into_split();
    ProxyClient {
        reader: BufReader::new(read_half),
        writer: write_half,
    }
}

#[tokio::test]
async fn call_round_trips_small_response_happy_path() {
    let (client_stream, server_stream) = UnixStream::pair().expect("unix pair");
    let mut client = proxy_client(client_stream);
    let server = tokio::spawn(async move {
        let (read_half, mut write_half) = server_stream.into_split();
        let mut reader = BufReader::new(read_half);
        let mut request_line = String::new();
        reader
            .read_line(&mut request_line)
            .await
            .expect("read request");
        assert!(
            request_line.contains("\"ping\""),
            "unexpected request: {request_line}"
        );
        let mut response = serde_json::to_string(&Response::ok_empty()).expect("serialize");
        response.push('\n');
        write_half
            .write_all(response.as_bytes())
            .await
            .expect("write response");
    });

    let response = client.call(ping_request()).await.expect("ping response");
    server.await.expect("server task");
    response.into_result().expect("ping should succeed");
}

#[tokio::test]
async fn call_rejects_oversized_request_edge_case() {
    let (client_stream, _server_stream) = UnixStream::pair().expect("unix pair");
    let mut client = proxy_client(client_stream);
    let sql = "x".repeat(MAX_SESSION_LINE_BYTES);
    let err = client
        .call(Request::Exec { sql })
        .await
        .expect_err("oversized request should fail");
    assert!(
        matches!(err, crate::error::Error::InvalidInput(ref msg) if msg.contains("request exceeds size limit")),
        "unexpected error: {err}"
    );
}

#[tokio::test]
async fn call_rejects_oversized_response_regression() {
    let (client_stream, server_stream) = UnixStream::pair().expect("unix pair");
    let mut client = proxy_client(client_stream);
    let server = tokio::spawn(async move {
        let (read_half, mut write_half) = server_stream.into_split();
        let mut reader = BufReader::new(read_half);
        let mut request_line = String::new();
        reader
            .read_line(&mut request_line)
            .await
            .expect("read request");
        let mut response = "x".repeat(MAX_SESSION_LINE_BYTES + 1);
        response.push('\n');
        write_half
            .write_all(response.as_bytes())
            .await
            .expect("write oversized response");
    });

    let err = client
        .call(ping_request())
        .await
        .expect_err("oversized response should fail");
    server.await.expect("server task");
    assert!(
        matches!(err, crate::error::Error::InvalidInput(ref msg) if msg.contains("response exceeds size limit")),
        "unexpected error: {err}"
    );
}

#[tokio::test]
async fn timing_conn_exec_times_out_when_server_stalls() {
    use crate::driver::{DbClient, IoProfile, TimingConn};
    use std::sync::{Arc, Mutex};
    use std::time::Duration;

    let (client_stream, server_stream) = UnixStream::pair().expect("unix pair");
    let server = tokio::spawn(async move {
        let (read_half, _write_half) = server_stream.into_split();
        let mut reader = BufReader::new(read_half);
        let mut line = String::new();
        // Read the exec request, then never reply (simulates a wedged daemon / SQL Server).
        let _ = reader.read_line(&mut line).await;
        tokio::time::sleep(Duration::from_secs(30)).await;
    });

    let proxy = proxy_client(client_stream);
    let io = Arc::new(Mutex::new(IoProfile::default()));
    let mut conn = TimingConn::new(DbClient::Proxy(proxy), io, 0);
    conn.set_command_timeout(Duration::from_millis(50));

    let err = conn
        .exec("SELECT 1")
        .await
        .expect_err("a stalled server must hit the command timeout");
    assert!(
        err.to_string().contains("timed out"),
        "unexpected error: {err}"
    );
    server.abort();
}

/// A peer streaming an over-limit line WITHOUT a newline must be cut off at
/// the cap (bounded read), not buffered until it sends one.
#[tokio::test]
async fn call_caps_no_newline_flood_regression() {
    let (client_stream, server_stream) = UnixStream::pair().expect("unix pair");
    let mut client = proxy_client(client_stream);
    let server = tokio::spawn(async move {
        let (read_half, mut write_half) = server_stream.into_split();
        let mut reader = BufReader::new(read_half);
        let mut request_line = String::new();
        reader
            .read_line(&mut request_line)
            .await
            .expect("read request");
        // Over-limit bytes with NO trailing newline; keep the stream open.
        let flood = "x".repeat(MAX_SESSION_LINE_BYTES + 2);
        write_half
            .write_all(flood.as_bytes())
            .await
            .expect("write flood");
        tokio::time::sleep(std::time::Duration::from_secs(30)).await;
    });

    let err = client
        .call(ping_request())
        .await
        .expect_err("flood without newline must fail at the cap");
    assert!(
        matches!(err, crate::error::Error::InvalidInput(ref msg) if msg.contains("response exceeds size limit")),
        "unexpected error: {err}"
    );
    server.abort();
}
