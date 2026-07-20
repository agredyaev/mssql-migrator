# Security Policy

## Supported Versions

Only the latest 1.x release receives security fixes.

| Version | Supported          |
| ------- | ------------------ |
| 1.x     | :white_check_mark: |
| < 1.0   | :x:                |

## Reporting a Vulnerability

Report vulnerabilities privately via GitHub Security Advisories
("Report a vulnerability" on this repository's Security tab). Do not open a
public issue for security reports.

You can expect an acknowledgement within 7 days. Confirmed issues are fixed in
the next patch release; the advisory is published after the fix ships.

Secrets note: local configuration lives in `.env` (database password, daemon
session token). `.env` is gitignored — never commit a real one.
