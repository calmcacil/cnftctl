# Release Notes And Evidence: v0.2.0

## Identity

- Version/tag: `v0.2.0`
- Candidate commit: `a2df7e38f77dba3b4dc236f7c3818c0b37749804`
- amd64 package: `cnftctl_0.2.0_amd64.deb` (`1713732` bytes)
- amd64 SHA-256: `58115490c323bfcee8929774f41be05eafce7b079d32f4b1099c9603258b80d0`
- arm64 package: `cnftctl_0.2.0_arm64.deb` (`1455656` bytes)
- arm64 SHA-256: `52144c150dfadb5faa78c97f27f4e52bc0fb7ae38c1c3338a6c9b41397d037af`
- Candidate run: [29291333684](https://github.com/calmcacil/cnftctl/actions/runs/29291333684)
- Tester and UTC date: project owner/Codex, 2026-07-13

## Highlights

- Native Debian 13 packages are published for amd64 and arm64.
- amd64 remains production-supported and completed the full exact-package live
  activation, rollback, reboot/recovery, DDNS, Docker, upgrade, and uninstall
  checklist.
- arm64 is explicitly experimental, unvalidated on a disposable live host,
  unsupported for production/security purposes, and used at the operator's
  own risk.
- Package metadata, installed manifests, pre-install guards, and ELF binaries
  are architecture-specific and accept only amd64 or arm64.
- CI and candidate builds run on native GitHub-hosted runners for both
  architectures.
- Each package has an architecture-named SPDX SBOM and matching GitHub
  provenance/SBOM attestations.
- Promotion verifies the closed five-file inventory and publishes unchanged
  candidate bytes without rebuilding.
- Runtime reporting distinguishes production amd64 from experimental arm64
  through structured support-tier details.
- The README now carries a prominent personal-use and operator-trust
  disclaimer.

Firewall rendering, nftables policy, systemd units, rollback semantics,
transaction logic, and Docker/DDNS behavior are unchanged by the architecture
expansion.

## Compatibility And Limitations

- Production support remains exactly Debian 13 amd64 with systemd and nftables.
- Debian 13 arm64 packages are experimental and carry no production or
  security-support guarantee.
- Docker external IPv6 traffic remains `NOT EXERCISED`; generated IPv6 rules
  pass exact nftables syntax validation.
- Debian Docker 26 rejects the proposed `firewall-backend` directive, so no
  daemon configuration was written or restarted during validation.
- The project remains a narrow manager for the application-owned `inet hostfw`
  table, not a general firewall manager.

## Evidence

- Native amd64 and arm64 automated gates: `PASS`.
- Offline verification, reproducibility, lifecycle tests, lintian, SBOMs, and
  attestations: `PASS`.
- Exact amd64 HOST_A/HOST_B validation: `PASS`.
- Docker IPv4 external WAN gate and coexistence: `PASS`.
- Docker IPv6 external traffic: `NOT EXERCISED`.
- Live arm64 firewall validation: `NOT EXERCISED`.
- Publication and public-download verification: pending tag/promotion.

Detailed sanitized evidence is in `docs/validation-record-0.2.0.md` and release
evidence issue #18. Raw host logs and archived audit state are retained
privately; credentials, hostnames, addresses, and trust values are excluded.
