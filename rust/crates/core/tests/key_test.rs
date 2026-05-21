use migrator_core::domain::ObjectKey;

#[test]
fn normalized_key() {
    let k = ObjectKey::new("Reporting", "Views", "Monthly");
    assert_eq!(k.as_str(), "reporting/views/monthly");
}
