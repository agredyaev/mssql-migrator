//! [`RowData`] and [`Cell`] — typed SQL row and column value types.

use serde::{Deserialize, Serialize};

use crate::error::{Error, Result};

/// Single column value from a database result row.
#[derive(Clone, Debug, Serialize, Deserialize)]
pub enum Cell {
    /// Null column value.
    #[serde(rename = "n")]
    Null,
    /// String column value.
    #[serde(rename = "s")]
    Str(String),
    /// Binary column value.
    #[serde(rename = "b")]
    Bytes(Vec<u8>),
}

/// Ordered column cells from a single database row.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
pub struct RowData {
    /// Ordered column cells corresponding to the query result columns.
    pub cells: Vec<Cell>,
}

impl RowData {
    /// Returns the string value at column `idx`, or `None` if absent or not a string cell.
    pub fn get_str(&self, idx: usize) -> Option<&str> {
        match self.cells.get(idx)? {
            Cell::Str(s) => Some(s.as_str()),
            _ => None,
        }
    }

    /// Returns the byte slice at column `idx`, or `None` if absent or not a bytes cell.
    pub fn get_bytes(&self, idx: usize) -> Option<&[u8]> {
        match self.cells.get(idx)? {
            Cell::Bytes(b) => Some(b.as_slice()),
            _ => None,
        }
    }

    /// Returns the integer value at column `idx`, or `None` if absent or unparseable.
    pub fn get_i32(&self, idx: usize) -> Option<i32> {
        if let Some(s) = self.get_str(idx) {
            return s.trim().parse().ok();
        }
        let b = self.get_bytes(idx)?;
        std::str::from_utf8(b).ok()?.trim().parse().ok()
    }
}

/// Converts a `tiberius::Row` into a `RowData` by extracting each column cell.
///
/// Fails closed: a column whose type no decode rung understands is an error,
/// not a silent `Cell::Null` — only a genuine SQL `NULL` maps to `Cell::Null`.
pub fn from_tiberius(row: &tiberius::Row) -> Result<RowData> {
    let n = row.columns().len();
    let mut cells = Vec::with_capacity(n);
    for i in 0..n {
        cells.push(cell_from(row, i)?);
    }
    Ok(RowData { cells })
}

/// `try_get` yields `Ok(None)` only when the requested type matches the column
/// and the value is NULL; a type mismatch is `Err`. Recording `Ok(None)` lets a
/// NULL of a supported type stay `Cell::Null` while an unsupported type fails.
fn take<'a, T: tiberius::FromSql<'a>>(
    row: &'a tiberius::Row,
    i: usize,
    saw_null: &mut bool,
) -> Option<T> {
    match row.try_get::<T, _>(i) {
        Ok(Some(v)) => Some(v),
        Ok(None) => {
            *saw_null = true;
            None
        }
        Err(_) => None,
    }
}

fn cell_from(row: &tiberius::Row, i: usize) -> Result<Cell> {
    let mut null = false;
    if let Some(b) = take::<&[u8]>(row, i, &mut null) {
        return Ok(Cell::Bytes(b.to_vec()));
    }
    if let Some(s) = take::<&str>(row, i, &mut null) {
        return Ok(Cell::Str(s.to_string()));
    }
    if let Some(b) = take::<bool>(row, i, &mut null) {
        return Ok(Cell::Str(if b { "1".into() } else { "0".into() }));
    }
    if let Some(n) = take::<i16>(row, i, &mut null) {
        return Ok(Cell::Str(n.to_string()));
    }
    if let Some(n) = take::<i32>(row, i, &mut null) {
        return Ok(Cell::Str(n.to_string()));
    }
    if let Some(n) = take::<i64>(row, i, &mut null) {
        return Ok(Cell::Str(n.to_string()));
    }
    if let Some(n) = take::<u8>(row, i, &mut null) {
        return Ok(Cell::Str(n.to_string()));
    }
    if null {
        return Ok(Cell::Null);
    }
    Err(Error::Sql(format!(
        "unsupported column type {:?} for column {} at index {i}",
        row.columns().get(i).map(tiberius::Column::column_type),
        row.columns().get(i).map_or("?", tiberius::Column::name),
    )))
}
