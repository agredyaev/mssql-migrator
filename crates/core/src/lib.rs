//! MSSQL reporting migrator core library.
#![warn(missing_docs)]
#![cfg_attr(not(test), deny(clippy::unwrap_used, clippy::expect_used))]

pub mod apply;
pub mod audit;
pub mod buildinfo;
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
pub mod plan;
pub mod scaffold;
pub mod scan;
pub mod session;
pub mod sql;
pub mod sql_ident;
pub mod timings;

pub use config::{Config, ConfigCold};
pub use error::{Error, Result};
pub use timings::PhaseTimings;
