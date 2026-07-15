//! Development-only helpers: footprint baselines, criterion/pprof, bench fixtures.
//!
//! Not a dependency of `rmig`, `rmigd`, or production `migrator-core` callers.

#![forbid(unsafe_code)]
pub mod bench;
pub mod perf;
pub mod pprof;
