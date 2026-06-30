#![allow(missing_docs)]

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
        match pprof::ProfilerGuard::new(self.frequency) {
            Ok(guard) => self.guard = Some(guard),
            Err(err) => eprintln!("pprof ProfilerGuard::new failed: {err}"),
        }
    }

    fn stop_profiling(&mut self, _benchmark_id: &str, benchmark_dir: &Path) {
        let Some(guard) = self.guard.take() else {
            return;
        };
        if let Err(err) = std::fs::create_dir_all(benchmark_dir) {
            eprintln!(
                "create profile dir {} failed: {err}",
                benchmark_dir.display()
            );
            return;
        }
        let output_path = benchmark_dir.join("flamegraph.svg");
        let file = match File::create(&output_path) {
            Ok(file) => file,
            Err(err) => {
                eprintln!("create {} failed: {err}", output_path.display());
                return;
            }
        };
        let mut options = Options::default();
        let report = match guard.report().build() {
            Ok(report) => report,
            Err(err) => {
                eprintln!("pprof report build failed: {err}");
                return;
            }
        };
        if let Err(err) = report.flamegraph_with_options(file, &mut options) {
            eprintln!("write {} failed: {err}", output_path.display());
        }
    }
}
