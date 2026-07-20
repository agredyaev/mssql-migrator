//! The daemon `USE` runs after the bounded handshake but before TimingConn's
//! per-command timeout exists; a wedged daemon must not hang the CLI there.

#![cfg(unix)]

use std::time::Duration;

use migrator_core::session::connect_daemon;
use migrator_core::Config;
use tokio::io::{AsyncBufReadExt, AsyncWriteExt, BufReader};

/// Stub daemon: answers the Ping handshake, then never replies to the USE.
#[tokio::test]
async fn use_statement_times_out_against_wedged_daemon_regression() {
    let dir = tempfile::tempdir().expect("tempdir");
    let sock = dir.path().join("stub.sock");
    let listener = tokio::net::UnixListener::bind(&sock).expect("bind");
    let server = tokio::spawn(async move {
        let (stream, _) = listener.accept().await.expect("accept");
        let (read_half, mut write_half) = stream.into_split();
        let mut reader = BufReader::new(read_half);
        let mut line = String::new();
        // Ping request → ok. (No token in cfg ⇒ no Auth message first.)
        reader.read_line(&mut line).await.expect("read ping");
        write_half
            .write_all(b"{\"ok\":true,\"error\":\"\",\"rows\":[]}\n")
            .await
            .expect("pong");
        // Read the Exec (USE ...) and never answer.
        line.clear();
        let _ = reader.read_line(&mut line).await;
        tokio::time::sleep(Duration::from_secs(30)).await;
    });

    let mut cfg = Config::default();
    cfg.database = "somedb".into();
    cfg.command_timeout = Duration::from_millis(100);
    let started = std::time::Instant::now();
    let err = match connect_daemon(&sock.to_string_lossy(), &cfg).await {
        Err(e) => e,
        Ok(_) => panic!("wedged USE must time out"),
    };
    assert!(
        started.elapsed() < Duration::from_secs(5),
        "must fail fast, took {:?}",
        started.elapsed()
    );
    assert!(
        err.to_string().contains("timed out"),
        "unexpected error: {err}"
    );
    server.abort();
}
