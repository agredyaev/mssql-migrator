use std::path::PathBuf;
use std::process::Command;

fn main() {
    let manifest_dir = std::env::var("CARGO_MANIFEST_DIR")
        .map(PathBuf::from)
        .unwrap_or_else(|_| PathBuf::from("."));
    let repo_root = manifest_dir.join("../..");

    let commit = git_short_head(&repo_root).unwrap_or_else(|| "unknown".into());
    println!("cargo:rustc-env=RMIG_COMMIT={commit}");
}

fn git_short_head(repo: &PathBuf) -> Option<String> {
    let out = Command::new("git")
        .args(["-C"])
        .arg(repo)
        .args(["rev-parse", "--short", "HEAD"])
        .output()
        .ok()?;
    if !out.status.success() {
        return None;
    }
    let hash = String::from_utf8(out.stdout).ok()?.trim().to_string();
    if hash.is_empty() {
        None
    } else {
        Some(hash)
    }
}
