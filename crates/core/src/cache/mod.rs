//! L1 filesystem caching for database catalog metadata snapshots.
//!
//! ### Purpose
//! Avoids expensive SQL Server catalog inspection round-trips by storing serializable structures
//! on disk during incremental, high-frequency planning runs.
//!
//! ### Architectural Context
//! - **Inputs**: Local cache directory layouts, serialized snapshot bytes.
//! - **Outputs**: deserialized memory structures.
//! - **Boundaries**: Writes under the configured path (e.g. `.rmig/cache/`).
//!
//! ### Nominal Flow
//! 1. Open local cache repository.
//! 2. Retrieve or write database metadata snapshots as required.
//! 3. Invalidate/purge file trees upon schema updates.
//!
//! ### Off-Nominal & Failure Containment
//! - **Corrupted Snapshots**: Automatically disregards unparseable snapshot files, logs the warning, and falls back to a full database catalog rebuild.

/// L1 filesystem cache for database catalog metadata.
pub mod l1;
