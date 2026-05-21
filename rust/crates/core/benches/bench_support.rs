//! Shared helpers for migrator-core Criterion/dhat benches (each bench crate uses a subset).

#![expect(dead_code)]

#[path = "bench_common.rs"]
mod bench_common;
#[path = "bench_scan.rs"]
mod bench_scan;
#[path = "bench_skip.rs"]
mod bench_skip;
#[path = "bench_table.rs"]
mod bench_table;

pub use bench_scan::{scan_fixture_workspace, temp_scan_root};
pub use bench_skip::skip_heavy_workspace;
pub use bench_table::table_heavy_workspace;
