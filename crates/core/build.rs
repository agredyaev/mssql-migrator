use std::path::PathBuf;
use std::process::Command;

fn main() {
    let manifest_dir =
        PathBuf::from(std::env::var("CARGO_MANIFEST_DIR").expect("CARGO_MANIFEST_DIR"));
    let repo_root = manifest_dir.join("../..");
    let version_path = repo_root.join("VERSION");

    println!("cargo:rerun-if-changed={}", version_path.display());

    let version = std::fs::read_to_string(&version_path)
        .unwrap_or_else(|_| "0.0.0-dev".into())
        .trim()
        .to_string();
    let version = if version.is_empty() {
        "0.0.0-dev".into()
    } else {
        version
    };
    println!("cargo:rustc-env=RMIG_VERSION={version}");

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
