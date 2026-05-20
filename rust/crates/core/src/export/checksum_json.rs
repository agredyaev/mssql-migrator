use base64::{engine::general_purpose::STANDARD, Engine};
use serde::{Deserialize, Deserializer, Serializer};

pub fn serialize<S>(bytes: &[u8; 32], s: S) -> Result<S::Ok, S::Error>
where
    S: Serializer,
{
    s.serialize_str(&STANDARD.encode(bytes))
}

pub fn deserialize<'de, D>(d: D) -> Result<[u8; 32], D::Error>
where
    D: Deserializer<'de>,
{
    let s = String::deserialize(d)?;
    let raw = STANDARD
        .decode(s.as_bytes())
        .map_err(serde::de::Error::custom)?;
    raw.try_into()
        .map_err(|_| serde::de::Error::custom("checksum must be 32 bytes"))
}
