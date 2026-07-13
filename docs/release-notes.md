# Release Notes And Evidence: 0.1.0 Candidate

## Identity

- Version/tag: `0.1.0` candidate; `v0.1.0` is not yet created
- Commit: `ee7ab0fd6932bafe1c22b684ec72e27e50803f94`
- Artifact: `cnftctl_0.1.0_amd64.deb` (`1712440` bytes)
- Artifact SHA-256: `93966559a326522a984cc8dcd36a062d5f4931a8c51cacecdc847664b277b198`
- CI run: [Release Candidate Build 29282503578](https://github.com/calmcacil/cnftctl/actions/runs/29282503578)
- Tester and UTC date: project owner/Codex, 2026-07-13

## Scope

This release provides the narrow `inet hostfw` manager for Debian 13 amd64, including desired YAML configuration, immutable generations, dedicated systemd activation, mandatory dead-man rollback and boot reconciliation, SSH session coverage checks with audited override, optional DDNS SSH sets, and optional strict Docker WAN gating.

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
- Docker external IPv6 traffic was not exercised; generated IPv6 rules passed exact nftables syntax validation.
- Debian Docker 26 rejects the proposed `firewall-backend` directive, so a supported backend write was not exercised and no daemon file was changed.
- Provider KVM login was confirmed and provides a shell independent of SSH and nftables. Operators without provider console/rescue access necessarily depend on rollback completing successfully.
- The tag, promotion, and clean public-download verification remain pending.

## Evidence

- CI checks: `PASS`; candidate run linked above.
- Offline package verification, reproducibility, and lintian: `PASS`.
- First-install timeout/table deletion: `PASS`.
- Confirm and prior-generation rollback: `PASS`.
- Initiating-session death and reboot reconciliation: `PASS`.
- SSH override audit: `PASS`.
- DDNS refresh, expiry/freshness, and timer reconciliation: `PASS`.
- Foreign nftables and Docker table coexistence: `PASS`.
- Strict Docker IPv4 WAN gate: `PASS`; external IPv6 `NOT EXERCISED`.
- Fresh install, active reinstall, purge-preserved reinstall, upgrade, and uninstall: `PASS`.
- Corrupt preserved-state refusal before unpack: `PASS`.
- JSON schema and exit codes: `PASS`.
- Security, vulnerability, license, notice, and sanitization review: `PASS`.
- SBOM and build provenance: `PASS`; two attestations refer to the exact package digest.
- Provider KVM recovery path: `PASS`; recorded in release evidence issue #8.

Detailed results and remaining gates are in `docs/validation-record-0.1.0.md`
and `docs/production-readiness.md`. Retained raw command output and journals
are indexed in release evidence issue #8 with sensitive infrastructure values
excluded under the repository sanitization policy.
