use std::fmt;

#[derive(Debug)]
pub enum Error {
    Config(String),
    InvalidInput(String),
    Io(std::io::Error),
    Sql(String),
    PlanBlocked,
    LockTimeout,
    Other(anyhow::Error),
}

pub type Result<T> = std::result::Result<T, Error>;

pub const EXIT_OK: i32 = 0;
pub const EXIT_GENERAL: i32 = 1;
pub const EXIT_CONFIG: i32 = 2;
pub const EXIT_CONN: i32 = 3;
pub const EXIT_CHECKSUM: i32 = 4;
pub const EXIT_SQL: i32 = 5;
pub const EXIT_VALIDATION: i32 = 6;
pub const EXIT_LOCK_TIMEOUT: i32 = 7;
pub const EXIT_INVALID_INPUT: i32 = 8;
pub const EXIT_CRITICAL: i32 = 9;
pub const EXIT_PLAN_BLOCKED: i32 = 10;

impl Error {
    pub fn exit_code(&self) -> i32 {
        match self {
            Self::Config(_) => EXIT_CONFIG,
            Self::InvalidInput(_) => EXIT_INVALID_INPUT,
            Self::PlanBlocked => EXIT_PLAN_BLOCKED,
            Self::LockTimeout => EXIT_LOCK_TIMEOUT,
            Self::Sql(m) => {
                let lower = m.to_lowercase();
                if lower.contains("connect ") || lower.starts_with("connect") {
                    EXIT_CONN
                } else {
                    EXIT_SQL
                }
            }
            Self::Io(_) => EXIT_GENERAL,
            Self::Other(_) => EXIT_GENERAL,
        }
    }
}

impl fmt::Display for Error {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::Config(m) => write!(f, "configuration error: {m}"),
            Self::InvalidInput(m) => write!(f, "{m}"),
            Self::Io(e) => write!(f, "{e}"),
            Self::Sql(m) => write!(f, "{m}"),
            Self::PlanBlocked => write!(f, "plan is blocked"),
            Self::LockTimeout => write!(f, "lock timeout"),
            Self::Other(e) => write!(f, "{e}"),
        }
    }
}

impl std::error::Error for Error {}

impl From<std::io::Error> for Error {
    fn from(e: std::io::Error) -> Self {
        Self::Io(e)
    }
}

impl From<anyhow::Error> for Error {
    fn from(e: anyhow::Error) -> Self {
        Self::Other(e)
    }
}
