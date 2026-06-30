//! Compact numeric codes for SQL object kinds (`u8`).
//!
//! ### Purpose
//! [`KindCode`] encodes the 9 supported SQL object kinds as `#[repr(u8)]`
//! so they can be stored densely in script metadata and compared without
//! string allocation. `parse_kind` resolves a kind string to its code;
//! `kind_code` is the shorthand version.

/// Numeric kind code for the `types` category (1).
pub const KIND_TYPES: u8 = 1;
/// Numeric kind code for the `sequences` category (2).
pub const KIND_SEQUENCES: u8 = 2;
/// Numeric kind code for the `tables` category (3).
pub const KIND_TABLES: u8 = 3;
/// Numeric kind code for the `synonyms` category (4).
pub const KIND_SYNONYMS: u8 = 4;
/// Numeric kind code for the `indexes` category (5).
pub const KIND_INDEXES: u8 = 5;
/// Numeric kind code for the `views` category (6).
pub const KIND_VIEWS: u8 = 6;
/// Numeric kind code for the `functions` category (7).
pub const KIND_FUNCTIONS: u8 = 7;
/// Numeric kind code for the `procedures` category (8).
pub const KIND_PROCEDURES: u8 = 8;
/// Numeric kind code for the `triggers` category (9).
pub const KIND_TRIGGERS: u8 = 9;

/// Compact numeric code identifying an SQL object kind.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
#[repr(u8)]
pub enum KindCode {
    /// Kind string did not match any supported category.
    Unknown = 0,
    /// User-defined type objects.
    Types = KIND_TYPES,
    /// Sequence objects.
    Sequences = KIND_SEQUENCES,
    /// Table objects.
    Tables = KIND_TABLES,
    /// Synonym objects.
    Synonyms = KIND_SYNONYMS,
    /// Index objects.
    Indexes = KIND_INDEXES,
    /// View objects.
    Views = KIND_VIEWS,
    /// Function objects.
    Functions = KIND_FUNCTIONS,
    /// Stored procedure objects.
    Procedures = KIND_PROCEDURES,
    /// Trigger objects.
    Triggers = KIND_TRIGGERS,
}

impl KindCode {
    /// Returns the `KindCode` for the given kind string, or `Unknown` if unrecognised.
    pub fn parse_kind(kind: &str) -> Self {
        match kind {
            "types" => Self::Types,
            "sequences" => Self::Sequences,
            "tables" => Self::Tables,
            "synonyms" => Self::Synonyms,
            "indexes" => Self::Indexes,
            "views" => Self::Views,
            "functions" => Self::Functions,
            "procedures" => Self::Procedures,
            "triggers" => Self::Triggers,
            _ => Self::Unknown,
        }
    }

    /// Returns the numeric representation of this code.
    pub fn as_u8(self) -> u8 {
        self as u8
    }
}

/// Returns the numeric kind code for the given kind string.
pub fn kind_code(kind: &str) -> u8 {
    KindCode::parse_kind(kind).as_u8()
}

/// Returns `true` if `code` represents a module kind (views, functions, procedures, triggers).
pub fn is_module_kind_code(code: u8) -> bool {
    matches!(
        code,
        KIND_VIEWS | KIND_FUNCTIONS | KIND_PROCEDURES | KIND_TRIGGERS
    )
}

/// Returns `true` if `code` represents a transactional kind (tables, indexes, types, sequences, synonyms).
pub fn is_transactional_kind_code(code: u8) -> bool {
    matches!(
        code,
        KIND_TABLES | KIND_INDEXES | KIND_TYPES | KIND_SEQUENCES | KIND_SYNONYMS
    )
}
