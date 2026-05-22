//! Git repository metadata extraction, merge-base computation, and diff analysis.
//!
//! ### Purpose
//! Extracts Git tracking information (e.g. merge base, changed file list, commit logs) to enable
//! fast-path incremental inspections by isolating changed schema files since the last deployment.
//!
//! ### Architectural Context
//! - **Inputs**: Workspace path, target remote branch reference (e.g. `origin/main`).
//! - **Outputs**: List of altered layout file paths, Git metadata structs (`GitMeta`).
//! - **Boundaries**: Spawns standard OS git child processes.
//!
//! ### Nominal Flow
//! 1. Verify existence of Git repository inside or above the workspace root (`has_git_repo`).
//! 2. Resolve the common ancestor commit reference against the target branch (`merge_base`).
//! 3. Extract the list of filenames altered since that commit (`changed_paths_from_git`).
//!
//! ### Off-Nominal & Failure Containment
//! - **Git Command Missing / Un-tracked Tree**: If git is not installed or the workspace has no repository tracking, falls back safely to full database catalog scans, avoiding migration failure.

mod diff;
mod log;
mod repo;

pub use diff::{
    changed_paths_from_git, diff_name_only, merge_base, merge_base_paths, parse_name_only,
};
pub use log::{batched_git_log, git_info_file, normalize_git_path, parse_commit_line, GitMeta};
pub use repo::{find_repo_root, has_git_repo, sql_path_prefix, sql_prefix_in_repo};
