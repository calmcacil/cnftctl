# Release Notes And Evidence Template

## Identity

- Version/tag: `vX.Y.Z`
- Commit: `FULL_SHA`
- Artifact: `cnftctl_X.Y.Z_amd64.deb`
- Artifact SHA-256: `RECORD_AT_RELEASE`
- CI run: `RECORD_AT_RELEASE`
- Tester and UTC date: `RECORD_AT_RELEASE`

## Scope

This release provides the narrow `inet hostfw` manager for Debian 13 amd64, including desired YAML configuration, immutable generations, dedicated systemd activation, mandatory dead-man rollback and boot reconciliation, SSH session coverage checks with audited override, optional DDNS SSH sets, and optional strict Docker WAN gating.

This template is not release evidence. Replace every placeholder with results from the exact canonical `cnftctl_X.Y.Z_amd64.deb` package on Debian 13 amd64 before publication.

The supported installation unit is the verified Debian package. Installation enables only reconciliation and does not activate firewall policy, enable DDNS, or restart Docker.

## Compatibility And Limitations

- Supported only on Debian 13 amd64 with systemd and nftables.
- Not a general nftables manager; foreign tables remain outside project ownership.
- Docker support gates published traffic but does not configure containers or restart Docker.
- DDNS trusts configured DNS results and supports only `/56` or `/64` IPv6 derivation.
- `doctor` currently performs the same checks as `status`.
- The rollback timeout is fixed at 120 seconds.
- Manual reference deployment and the internal bundle are behavior/build references, not supported installation paths.
- `transactions list` reports pending transactions, not historical completed transactions.
- Support remains withheld until all exact-artifact evidence below passes; do not substitute source-tree or code-fix results.

## Evidence

- CI checks: `PASS/FAIL` with URL.
- Offline package verification, reproducibility, and lintian: `PASS/FAIL` with output attachment.
- First-install timeout/table deletion: `PASS/FAIL`.
- Confirm and prior-generation rollback: `PASS/FAIL`.
- Initiating-session death and reboot reconciliation: `PASS/FAIL`.
- SSH override audit: `PASS/FAIL`.
- DDNS refresh, expiry/freshness, and timer reconciliation: `PASS/FAIL`.
- Foreign nftables and Docker table coexistence: `PASS/FAIL`.
- Strict Docker WAN gate: `PASS/FAIL/NOT EXERCISED`.
- Fresh install, upgrade, and uninstall: `PASS/FAIL`.
- JSON schema and exit codes: `PASS/FAIL`.
- Security/license/notice review: `PASS/FAIL`.

Attach the completed `docs/manual-validation.md` record. Replace every placeholder before publication; do not publish this template as evidence.

Also attach a completed copy of `docs/validation-record.md` and show that every mandatory item in `docs/production-readiness.md` is closed for the same artifact SHA-256.
