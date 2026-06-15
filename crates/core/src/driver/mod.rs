//! Tiberius TDS client connection wrapper, query timing metrics, and session profiling.
//!
//! ### Purpose
//! Establishes low-level TCP/TDS connections with SQL Server, manages active database sessions, and
//! instruments query execution times to log metrics.
//!
//! ### Architectural Context
//! - **Inputs**: SQL query requests, connection settings.
//! - **Outputs**: DB Row vectors, query latency metrics.
//! - **Boundaries**: Uses a timing connection wrapper (`TimingConn`) to record network and DB execution times.
//!
//! ### Nominal Flow
//! 1. Open socket connection to SQL Server (`connect`).
//! 2. Execute SQL query.
//! 3. Collect row data and format to standard `RowData` structs.
//! 4. Log latency metrics via `TimingConn`.
//!
//! ### Off-Nominal & Failure Containment
//! - **Socket Timeout / Bad Credentials**: Fails safe and raises `Error::Sql` with the exact Tiberius exception.

pub mod db_client;
pub mod io_profile;
pub mod mssql;
pub mod mssql_auth;
pub mod mssql_query;
pub mod row;
pub mod timing;

pub use db_client::DbClient;
pub use io_profile::IoProfile;
pub use mssql::{connect, MssqlConn, RawClient};
pub use mssql_auth::select_auth_method;
pub use mssql_query::{exec, ping, query_all_results, query_tiberius};
pub use row::{from_tiberius, RowData};
pub use timing::TimingConn;
