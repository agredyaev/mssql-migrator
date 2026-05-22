//! In-process plan diff 5k (skip-heavy), CPU flamegraph via Criterion + pprof.

use std::hint::black_box;

use criterion::{criterion_group, criterion_main, Criterion};
use migrator_core::export::MigrationPlan;
use migrator_core::plan::compute_diff_into;
use migrator_core_dev::bench::skip::skip_heavy_workspace;
use migrator_core_dev::pprof::RmigPprofProfiler;

fn bench_plan_diff_skip_heavy_5000(c: &mut Criterion) {
    let (mut ws, catalog, checksums) = skip_heavy_workspace(5000);
    let mut plan = MigrationPlan::default();
    compute_diff_into(&mut ws, &catalog, &checksums, &mut plan).expect("warm");
    c.bench_function("plan_diff_skip_heavy_5000", |b| {
        b.iter(|| {
            let ms = compute_diff_into(
                black_box(&mut ws),
                black_box(&catalog),
                black_box(&checksums),
                black_box(&mut plan),
            )
            .expect("diff");
            black_box(ms);
        });
    });
}

criterion_group! {
    name = benches;
    config = Criterion::default().with_profiler(RmigPprofProfiler::new(1000));
    targets = bench_plan_diff_skip_heavy_5000
}
criterion_main!(benches);
