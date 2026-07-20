use super::*;
use crate::driver::row::{Cell, RowData};

#[test]
fn undecodable_checksum_is_repair_only_regression() {
    let mut row = RowData::default();
    row.cells.push(Cell::Str("dbo/views/v".into()));
    row.cells.push(Cell::Str("not-hex!".into()));
    let err = checksum_map_from_rows(std::slice::from_ref(&row), false)
        .expect_err("normal planning must reject corrupt audit history");
    assert_eq!(err.exit_code(), crate::error::EXIT_CHECKSUM);
    assert!(err.to_string().contains("dbo/views/v"), "{err}");

    let map = checksum_map_from_rows(std::slice::from_ref(&row), true)
        .expect("repair-checksum may replace the corrupt row");
    assert_eq!(
        map.get_normalized("dbo/views/v"),
        Some(&[0u8; 32]),
        "repair must receive a zero baseline"
    );
}

#[test]
fn parse_history_checksum_hex_string() {
    let mut row = RowData::default();
    row.cells.push(Cell::Str(
        "75fdafa30d217c791047a3d8bd5f36dd62548e04a5154e758355a51525b2f973".into(),
    ));
    let sum = parse_history_checksum(&row, 0).expect("hex checksum");
    assert_eq!(
        hex::encode(sum),
        "75fdafa30d217c791047a3d8bd5f36dd62548e04a5154e758355a51525b2f973"
    );
}

#[test]
fn live_definition_drift_flag_forces_module_update_regression() {
    let mut row = RowData::default();
    row.cells.push(Cell::Str("dbo/views/v".into()));
    row.cells.push(Cell::Str(
        "75fdafa30d217c791047a3d8bd5f36dd62548e04a5154e758355a51525b2f973".into(),
    ));
    row.cells.push(Cell::Str("1".into()));
    let map = checksum_map_from_rows_ws(&[row], false).expect("valid checksum row");
    assert!(map.has_live_definition_drift(&ObjectKey::from_normalized("dbo/views/v")));
}
