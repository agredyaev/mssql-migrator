//! Criterion 0.8 [`Profiler`](criterion::profiler::Profiler) backed by [`pprof`] flamegraphs.
//!
//! Upstream `pprof` optional `criterion` feature targets **criterion 0.5** only; this module
//! keeps workspace **criterion 0.8** and writes `flamegraph.svg` under Criterion's profile dir.

use std::fs::File;
use std::os::raw::c_int;
use std::path::Path;

use criterion::profiler::Profiler;
use pprof::flamegraph::Options;

pub struct RmigPprofProfiler {
    frequency: c_int,
    guard: Option<pprof::ProfilerGuard<'static>>,
}

impl RmigPprofProfiler {
    pub fn new(frequency: c_int) -> Self {
        Self {
            frequency,
            guard: None,
        }
    }
}

impl Profiler for RmigPprofProfiler {
    fn start_profiling(&mut self, _benchmark_id: &str, _benchmark_dir: &Path) {
        self.guard = Some(
            pprof::ProfilerGuard::new(self.frequency).expect("pprof ProfilerGuard::new"),
        );
    }

    fn stop_profiling(&mut self, _benchmark_id: &str, benchmark_dir: &Path) {
        let guard = self.guard.take().expect("profiler not started");
        std::fs::create_dir_all(benchmark_dir).expect("create profile dir");
        let output_path = benchmark_dir.join("flamegraph.svg");
        let file = File::create(&output_path)
            .unwrap_or_else(|e| panic!("create {}: {e}", output_path.display()));
        let mut options = Options::default();
        guard
            .report()
            .build()
            .expect("pprof report build")
            .flamegraph_with_options(file, &mut options)
            .expect("write flamegraph");
    }
}
