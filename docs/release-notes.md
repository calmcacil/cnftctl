# Release Notes And Evidence Template

## Identity

- Version/tag: `vX.Y.Z`
- Commit: `FULL_SHA`
- Artifact: `cnftctl-X.Y.Z-debian13-amd64.tar.gz`
- Artifact SHA-256: `RECORD_AT_RELEASE`
- CI run: `RECORD_AT_RELEASE`
- Tester and UTC date: `RECORD_AT_RELEASE`

## Scope

This release provides the narrow `inet hostfw` manager for Debian 13 amd64, including desired YAML configuration, immutable generations, dedicated systemd activation, mandatory dead-man rollback and boot reconciliation, SSH session coverage checks with audited override, optional DDNS SSH sets, and optional strict Docker WAN gating.

**Production status: NOT READY** until every evidence placeholder below is replaced with results from the exact canonical `cnftctl-X.Y.Z-debian13-amd64.tar.gz` artifact on Debian 13 amd64. This template describes the intended release contract and is not proof that any implementation or artifact passed.

The supported installation unit is the complete verified bundle. Installation does not activate firewall policy. Docker is never restarted automatically.

## Compatibility And Limitations

- Supported only on Debian 13 amd64 with systemd and nftables.
- Not a general nftables manager; foreign tables remain outside project ownership.
- Docker support gates published traffic but does not configure containers or restart Docker.
- DDNS trusts configured DNS results and supports only `/56` or `/64` IPv6 derivation.
- `doctor` currently performs the same checks as `status`.
- The rollback timeout is fixed at 120 seconds.
- Manual reference deployment is a behavior reference, not the supported bundle lifecycle.
- `transactions list` reports pending transactions, not historical completed transactions.
- Production use remains blocked until all exact-artifact evidence below passes; do not substitute source-tree or code-fix results.

## Evidence

- CI checks: `PASS/FAIL` with URL.
- Offline bundle verification: `PASS/FAIL` with output attachment.
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
