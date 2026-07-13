# Release Validation Record: v0.2.0

## Candidate Identity

| Field | Recorded Value |
| --- | --- |
| Version/tag | `v0.2.0` |
| Candidate commit SHA | `a2df7e38f77dba3b4dc236f7c3818c0b37749804` |
| amd64 artifact | `cnftctl_0.2.0_amd64.deb` (`1713732` bytes) |
| amd64 SHA-256 | `58115490c323bfcee8929774f41be05eafce7b079d32f4b1099c9603258b80d0` |
| arm64 artifact | `cnftctl_0.2.0_arm64.deb` (`1455656` bytes) |
| arm64 SHA-256 | `52144c150dfadb5faa78c97f27f4e52bc0fb7ae38c1c3338a6c9b41397d037af` |
| amd64 SBOM SHA-256 | `0abd920d7a297b82f43262ee33c9b92f4aeae528f1fb6030dd40c6150e7b2d27` |
| arm64 SBOM SHA-256 | `5d972d14bec45cdd32203e262cbb9f597e8287db1bd8cce05e317706991dc2ef` |
| Candidate workflow | [Release Candidate Build 29291333684](https://github.com/calmcacil/cnftctl/actions/runs/29291333684) |
| Go toolchain | Go `1.26.5` from `go.mod` |
| Artifact producer | Native GitHub-hosted amd64 and arm64 jobs on protected `main` |
| Candidate self-review | `PASS`, project owner/Codex, 2026-07-13 UTC |

The candidate contains exactly two packages, two architecture-named SPDX
SBOMs, and `release-checksums.txt`. Each package was built once by its native
job, attested there, and copied unchanged into the aggregate candidate.
Failed pre-candidate aggregation runs `29289852660` and `29290480224` are
rejected and are not publication inputs.

## Host Identity

The reusable disposable provider host was reset between HOST_A and HOST_B.
Private hostnames, public addresses, credentials, SSH client addresses, and
provider identifiers are excluded under the sanitization policy.

| Field | HOST_A | HOST_B |
| --- | --- | --- |
| Purpose | Base firewall lifecycle without Docker | Docker coexistence and WAN gating |
| Distribution | Debian 13 | Debian 13 |
| Architecture | `amd64` | `amd64` |
| Kernel | `6.12.95+deb13-amd64` | `6.12.95+deb13-amd64` |
| nftables | `1.1.3` | `1.1.3` |
| systemd | `257.13-1~deb13u1` | `257.13-1~deb13u1` |
| Docker | not installed | `26.1.5+dfsg1` |
| Console recovery | Provider KVM previously tested and reconfirmed available by the owner | Same host/path |
| Validation window | 2026-07-13 23:12–23:31 UTC | 2026-07-13 23:31–23:41 UTC |

## Automated And Offline Gate

| Check | Result | Evidence |
| --- | --- | --- |
| Formatting, tests, vet, race tests | `PASS` | Candidate run and PRs #15–#17 |
| Staticcheck and govulncheck | `PASS` | Candidate run |
| Native amd64 and arm64 delivery suites | `PASS` | Candidate run |
| Package reproducibility and lifecycle tests | `PASS` | Candidate run |
| Lintian | `PASS` | Candidate run |
| systemd, shellcheck, actionlint, staged nft syntax | `PASS` | Candidate run |
| Closed package/SBOM/checksum inventory | `PASS` | Aggregate job `86955880236` |
| Offline metadata, manifest, ELF, modes, checksums, version | `PASS` | Independent candidate download |
| Per-package provenance and SBOM attestations | `PASS` | Native candidate jobs |
| Exact arm64 binary execution on Debian 13 arm64 | `PASS` | Native CI and independent extraction |

## Exact amd64 Host Validation

| Area | Result | Notes |
| --- | --- | --- |
| Clean HOST_A baseline and exact install | `PASS` | Docker and prior cnftctl state removed; install activated no policy |
| Debian/version/architecture guards | `PASS` | Matching install accepted; mismatched architecture refused |
| Config defaults, permissions, validate/plan JSON | `PASS` | Mode `0600`, SSH open, no initial ports |
| First-install timeout | `PASS` | Only `inet hostfw` removed; selector absent; transaction rolled back |
| Confirmed generation and update timeout | `PASS` | Exact prior selector and nft output restored |
| Initiating SSH process termination | `PASS` | systemd timer completed rollback independently |
| Reboot reconciliation and boot load | `PASS` | Pending transaction rolled back; confirmed policy loaded |
| SSH coverage and audited override | `PASS` | Default refusal, reason requirement, audit, and rollback verified |
| DDNS A/AAAA `/56` and `/64` | `PASS` | Joint refresh and timer state verified |
| DDNS failure, atomic behavior, stale state | `PASS` | Reserved invalid hostname produced no partial replacement and stale metadata |
| DDNS disable and timer rollback | `PASS` | Explicit rollback restored timer; confirmed disable stopped it |
| Reports, drift, redaction, exit codes, journals | `PASS` | Healthy `0`, unhealthy inspection `1`, command error `2` |
| Pending/unsafe transaction refusal | `PASS` | Armed and symlinked state refused before unpack |
| Active reinstall preservation | `PASS` | Config, selector, policy, and audit history preserved |
| Previous-release upgrade | `PASS` | `v0.1.0` to exact `v0.2.0` preserved active state |
| Active uninstall refusal | `PASS` | Removal blocked while `inet hostfw` was active |
| Approved HOST_A uninstall | `PASS` | Delivery removed; state archived/preserved; managed table absent |
| Docker table and foreign-table coexistence | `PASS` | Apply, confirm, rollback, close, and owner-managed reboot verified |
| Docker IPv4 WAN gate | `PASS` | External probe timed out closed, served nginx open, timed out after close |
| Original public destination port | `PASS` | Published port `18080` controlled by matching public intent |
| Docker backend plan/write refusal | `PASS` | Debian Docker rejected directive; no file write or daemon restart |
| Docker-aware purge and reinstall | `PASS` | Container/tables survived; preserved state accepted; no policy activated |
| Docker IPv6 external traffic | `NOT EXERCISED` | No suitable external IPv6 Docker probe was available |

HOST_A ended with approved package removal, preserved-state archival and purge,
and no managed table before Docker installation. HOST_B ended with cnftctl
removed, validation state archived, the test container and foreign owner unit
removed, no managed table, Docker running, and no failed systemd units.

## Experimental arm64 Status

Native arm64 CI, package lifecycle testing, ELF/manifest verification, lintian,
and exact binary execution passed. Live firewall activation, rollback, reboot,
DDNS, Docker coexistence, and uninstall validation on a disposable arm64 host
with independent console access remain `NOT EXERCISED`. Arm64 therefore remains
experimental, unvalidated for production, without a production or security
support guarantee, and used at the operator's own risk.

## Final Decision

- [x] Every mandatory amd64 base-tier check passed for the same package SHA-256.
- [x] Docker limitations and all `NOT EXERCISED` items are disclosed.
- [x] Candidate identity matches build, host evidence, and publication inputs.
- [x] Candidate evidence and limitations received explicit self-review.
- [ ] Publicly downloaded bytes reverified after publication.

Decision: `APPROVE` for tagging and build-once promotion of candidate run
`29291333684`. The final checkbox is intentionally pending publication.
