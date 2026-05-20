//! CPU bench + flamegraph via Criterion profiler.
//!
//! ```bash
//! cd rust
//! cargo bench -p migrator-core --bench scan_digest -- --profile-time=2
//! ```

use criterion::{criterion_group, criterion_main, Criterion};
use migrator_core::domain::{empty_str, share, Script, ScriptKey, ScriptKind, Workspace};
use migrator_core::scan::layout_digest;
use pprof::criterion::{Output, PProfProfiler};

fn bench_layout_digest(c: &mut Criterion) {
    let mut ws = Workspace::default();
    let schema = share("s");
    let kind = share("views");
    for i in 0..5000 {
        let path = format!("db/s/views/obj_{i}.sql");
        let key = ScriptKey::from_path(&path);
        ws.scripts.insert(
            key.clone(),
            Script {
                key,
                kind: ScriptKind::Object,
                abs_path: share(&path),
                schema: schema.clone(),
                object_kind: kind.clone(),
                object_name: share(format!("obj_{i}")),
                checksum: None,
                git_hash: empty_str(),
                git_author: empty_str(),
                git_date: empty_str(),
                table_name: None,
                scaffold: false,
            },
        );
    }
    c.bench_function("layout_digest_5000", |b| b.iter(|| layout_digest(&ws)));
}

criterion_group! {
    name = benches;
    config = Criterion::default().with_profiler(PProfProfiler::new(100, Output::Flamegraph(None)));
    targets = bench_layout_digest
}
criterion_main!(benches);
