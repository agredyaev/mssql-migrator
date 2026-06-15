use serde::de::DeserializeOwned;

pub(super) fn read_report_object<T: DeserializeOwned>(
    data: &str,
    object_fields: &[&str],
) -> Result<T, serde_json::Error> {
    let value: serde_json::Value = serde_json::from_str(data)?;
    if !value.is_object() {
        return Err(report_shape_error("e2e report JSON must be an object"));
    }
    for field in object_fields {
        if let Some(nested) = value.get(field) {
            if !nested.is_object() {
                return Err(report_shape_error(format!(
                    "e2e report field `{field}` must be an object"
                )));
            }
        }
    }
    serde_json::from_value(value)
}

fn report_shape_error(message: impl Into<String>) -> serde_json::Error {
    serde_json::Error::io(std::io::Error::new(
        std::io::ErrorKind::InvalidData,
        message.into(),
    ))
}
