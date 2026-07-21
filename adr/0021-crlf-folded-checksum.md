# ADR-0021: CRLF-folded SHA-256 checksum (line-ending normalization)

Status: Accepted
Date: 2026-07-21

## Context

Object identity for skip-vs-changed (ADR-0002) is a checksum of the script body.
A repo cloned on Windows vs Linux, or with mixed `.gitattributes`, yields the same
logical SQL with different line endings (`\r\n` vs `\n`). A raw byte hash would
flag such files as "changed" and re-apply unchanged objects, or churn the plan.

## Decision

The checksum is SHA-256 over the body with CRLF folded to LF
(`crates/core/src/scan/parse_object.rs::content_checksum`): every `\r\n` is
hashed as `\n`; a lone `\r` is kept. Applied identically at scan time and when
`verified_body` re-hashes at apply, so the stored and recomputed checksums agree
regardless of the checkout's line-ending policy.

## Consequences

- Line-ending differences do not cause spurious "changed" classifications or
  plan churn across platforms/checkouts.
- The audited `history.checksum` is platform-independent for the same logical SQL.
- Two files differing only by CRLF vs LF hash equal — intended (same SQL). Files
  differing by actual content hash differently (correct).
- The live-definition fingerprint (ADR-0004/0006) is a separate hash of catalog
  metadata, not the body checksum; the two never mix.
