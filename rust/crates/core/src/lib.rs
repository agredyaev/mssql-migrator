//! MSSQL reporting migrator core (Rust port).

pub mod apply;
pub mod audit;
pub mod cache;
pub mod config;
pub mod db;
pub mod domain;
pub mod driver;
pub mod engine;
pub mod error;
pub mod export;
pub mod gate;
pub mod git;
pub mod lock;
pub mod perf;
pub mod plan;
pub mod scaffold;
pub mod scan;
// `scan::digest` re-exported for benches
pub mod session;
pub mod sql;
pub mod sql_ident;
pub mod timings;

pub use config::{Config, ConfigCold};
pub use error::{Error, Result};
pub use timings::PhaseTimings;
