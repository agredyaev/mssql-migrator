//! Synthetic workspaces for criterion/dhat (feature-gated per bench target).

pub mod common;

#[cfg(feature = "bench-skip")]
pub mod skip;

#[cfg(feature = "bench-transitions")]
pub mod table;

#[cfg(feature = "bench-scan")]
pub mod scan;
