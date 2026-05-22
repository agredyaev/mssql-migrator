use serde::{Deserialize, Serialize};

#[derive(Clone, Debug, Serialize, Deserialize)]
pub enum Cell {
    #[serde(rename = "n")]
    Null,
    #[serde(rename = "s")]
    Str(String),
    #[serde(rename = "b")]
    Bytes(Vec<u8>),
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
pub struct RowData {
    pub cells: Vec<Cell>,
}

impl RowData {
    pub fn get_str(&self, idx: usize) -> Option<&str> {
        match self.cells.get(idx)? {
            Cell::Str(s) => Some(s.as_str()),
            _ => None,
        }
    }

    pub fn get_bytes(&self, idx: usize) -> Option<&[u8]> {
        match self.cells.get(idx)? {
            Cell::Bytes(b) => Some(b.as_slice()),
            _ => None,
        }
    }

    pub fn get_i32(&self, idx: usize) -> Option<i32> {
        if let Some(s) = self.get_str(idx) {
            return s.trim().parse().ok();
        }
        let b = self.get_bytes(idx)?;
        std::str::from_utf8(b).ok()?.trim().parse().ok()
    }
}

pub fn from_tiberius(row: &tiberius::Row) -> RowData {
    let n = row.columns().len();
    let mut cells = Vec::with_capacity(n);
    for i in 0..n {
        if let Ok(Some(b)) = row.try_get::<&[u8], _>(i) {
            cells.push(Cell::Bytes(b.to_vec()));
        } else if let Ok(Some(s)) = row.try_get::<&str, _>(i) {
            cells.push(Cell::Str(s.to_string()));
        } else if let Ok(Some(b)) = row.try_get::<bool, _>(i) {
            cells.push(Cell::Str(if b { "1".into() } else { "0".into() }));
        } else if let Ok(Some(n)) = row.try_get::<i32, _>(i) {
            cells.push(Cell::Str(n.to_string()));
        } else if let Ok(Some(n)) = row.try_get::<i64, _>(i) {
            cells.push(Cell::Str(n.to_string()));
        } else if let Ok(Some(n)) = row.try_get::<u8, _>(i) {
            cells.push(Cell::Str(n.to_string()));
        } else {
            cells.push(Cell::Null);
        }
    }
    RowData { cells }
}
