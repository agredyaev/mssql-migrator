use tokio::io::{AsyncBufReadExt, AsyncWriteExt, BufReader};
use tokio::net::UnixStream;

use crate::session::limits::MAX_SESSION_LINE_BYTES;
use crate::session::protocol::{Request, Response};
use crate::session::proxy::ProxyClient;

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

    let response = client.call(Request::Ping).await.expect("ping response");
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
        .call(Request::Ping)
        .await
        .expect_err("oversized response should fail");
    server.await.expect("server task");
    assert!(
        matches!(err, crate::error::Error::InvalidInput(ref msg) if msg.contains("response exceeds size limit")),
        "unexpected error: {err}"
    );
}
