use std::process::Command;

pub fn short_hash() -> String {
    let out = Command::new("git")
        .args(["rev-parse", "--short", "HEAD"])
        .output();
    match out {
        Ok(o) if o.status.success() => String::from_utf8_lossy(&o.stdout).trim().to_string(),
        _ => "0000000".into(),
    }
}
