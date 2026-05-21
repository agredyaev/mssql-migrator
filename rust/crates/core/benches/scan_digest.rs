//! CPU bench + flamegraph via Criterion profiler.
//!
//! ```bash
//! cd rust
//! cargo bench -p migrator-core --bench scan_digest -- --profile-time=2
//! ```

use criterion::{criterion_group, criterion_main, Criterion};
use migrator_core::domain::{share, Script, ScriptKey, ScriptKind, Workspace};
use migrator_core::scan::layout_digest;

fn bench_layout_digest(c: &mut Criterion) {
    let mut ws = Workspace::default();
    for i in 0..5000 {
        let path = format!("db/s/views/obj_{i}.sql");
        ws.insert_script(Script {
            key: ScriptKey::from_path(&path),
            kind: ScriptKind::Object,
            abs_path: share(&path),
            checksum: None,
            scaffold: false,
        });
    }
    c.bench_function("layout_digest_5000", |b| b.iter(|| layout_digest(&ws)));
}

criterion_group! {
    name = benches;
    config = Criterion::default();
    targets = bench_layout_digest
}
criterion_main!(benches);
