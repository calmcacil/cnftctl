# Contributing

Contributions are accepted under the Apache License 2.0 in `LICENSE`.

By submitting a contribution, you certify that you have the right to submit it and agree that it is licensed to the project and recipients under Apache-2.0. Do not submit confidential material, third-party code without compatible licensing and attribution, or real infrastructure data.

## Workflow

1. Open an issue for firewall-semantic, compatibility, packaging, or security-sensitive changes before implementation.
2. Keep changes narrow and preserve the safety invariants in `SPEC.md` and `AGENTS.md`.
3. Add or update tests for behavior changes.
4. Run `sh ./scripts/check.sh`.
5. Update operator, support, release, and manual-validation documentation when behavior or evidence requirements change.
6. Use a Conventional Commit message where practical.

Examples and fixtures must use reserved documentation values such as `example.com`, `203.0.113.0/24`, `198.51.100.0/24`, and `2001:db8::/32`. Never submit tokens, credentials, private keys, personal allowlists, or non-public infrastructure identifiers.

Security vulnerabilities must not be reported in a public issue. Follow `SECURITY.md`.
