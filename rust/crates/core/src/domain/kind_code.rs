pub const KIND_TYPES: u8 = 1;
pub const KIND_SEQUENCES: u8 = 2;
pub const KIND_TABLES: u8 = 3;
pub const KIND_SYNONYMS: u8 = 4;
pub const KIND_INDEXES: u8 = 5;
pub const KIND_VIEWS: u8 = 6;
pub const KIND_FUNCTIONS: u8 = 7;
pub const KIND_PROCEDURES: u8 = 8;
pub const KIND_TRIGGERS: u8 = 9;

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
#[repr(u8)]
pub enum KindCode {
    Unknown = 0,
    Types = KIND_TYPES,
    Sequences = KIND_SEQUENCES,
    Tables = KIND_TABLES,
    Synonyms = KIND_SYNONYMS,
    Indexes = KIND_INDEXES,
    Views = KIND_VIEWS,
    Functions = KIND_FUNCTIONS,
    Procedures = KIND_PROCEDURES,
    Triggers = KIND_TRIGGERS,
}

impl KindCode {
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

    pub fn as_u8(self) -> u8 {
        self as u8
    }
}

pub fn kind_code(kind: &str) -> u8 {
    KindCode::parse_kind(kind).as_u8()
}

pub fn is_module_kind_code(code: u8) -> bool {
    matches!(
        code,
        KIND_VIEWS | KIND_FUNCTIONS | KIND_PROCEDURES | KIND_TRIGGERS
    )
}

pub fn is_transactional_kind_code(code: u8) -> bool {
    matches!(
        code,
        KIND_TABLES | KIND_INDEXES | KIND_TYPES | KIND_SEQUENCES | KIND_SYNONYMS
    )
}
