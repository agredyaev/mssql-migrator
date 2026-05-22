//! Automatic schema migration scaffolding and script directory creation.
//!
//! ### Purpose
//! Generates skeleton migration files containing templated DDL instructions (like `ALTER TABLE...`)
//! when un-migrated schema modifications are detected, helping operators write valid migration scripts.
//!
//! ### Architectural Context
//! - **Inputs**: `MigrationPlan` difference details, configured SQL layout roots.
//! - **Outputs**: Local directories and template SQL migration scripts on the filesystem.
//! - **Boundaries**: Operates locally by creating or modifying files under the schema folder.
//!
//! ### Nominal Flow
//! 1. Detect that the active layout plan contains blocked structural alterations.
//! 2. Resolve the target table directory layout in the schema tree.
//! 3. Build and write scaffold migration directory and files (`ensure`).
//!
//! ### Off-Nominal & Failure Containment
//! - **Filing exceptions**: If filesystem write permissions are lacking or directory pathing fails, captures the exception and aborts validation runs.

mod auto;
mod column_parser;
mod content;
mod dir;
mod ensure;
mod git;

pub use ensure::ensure;
