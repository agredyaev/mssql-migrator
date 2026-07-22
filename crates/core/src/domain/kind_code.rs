//! Compact numeric codes for SQL object kinds (`u8`).
//!
//! `kind_code` maps a kind string to its dense `u8` code so kinds can be
//! stored in script metadata and compared without string allocation.

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

/// Returns the numeric kind code for the given kind string.
pub fn kind_code(kind: &str) -> u8 {
    match kind {
        "types" => KIND_TYPES,
        "sequences" => KIND_SEQUENCES,
        "tables" => KIND_TABLES,
        "synonyms" => KIND_SYNONYMS,
        "indexes" => KIND_INDEXES,
        "views" => KIND_VIEWS,
        "functions" => KIND_FUNCTIONS,
        "procedures" => KIND_PROCEDURES,
        "triggers" => KIND_TRIGGERS,
        _ => 0,
    }
}

/// Returns `true` if `code` represents a module kind (views, functions, procedures, triggers).
pub fn is_module_kind_code(code: u8) -> bool {
    matches!(
        code,
        KIND_VIEWS | KIND_FUNCTIONS | KIND_PROCEDURES | KIND_TRIGGERS
    )
}
