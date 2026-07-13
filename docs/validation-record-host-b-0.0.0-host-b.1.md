# HOST_B Validation Record: 0.0.0-host-b.1

This is the sanitized evidence record for the completed HOST_B Docker and DDNS
run. The exact archive below includes a compatibility correction discovered on
HOST_B. It supersedes `0.0.0-host-a.11` as the current validation candidate but
is not approved for release: the final archive still requires a clean HOST_A
base-lifecycle run, CI provenance, an SBOM, and independent review.

## Candidate Identity

| Field | Recorded Value |
| --- | --- |
| Version/tag | `0.0.0-host-b.1` (validation version; no release tag) |
| Source identity | Dirty validation workspace based on commit `fc51df5612cb10605c13fca2f970dd1a9b4be10b`; includes all HOST_A fixes and the HOST_B Docker compatibility correction |
| Artifact filename | `cnftctl-0.0.0-host-b.1-debian13-amd64.tar.gz` |
| Artifact byte size | `2144464` |
| Artifact SHA-256 | `dbb18f7a05636f4cc11d65fb229027d67ea2d01dedd436dc37ea81b6d5059620` |
| Prior HOST_A artifact | `0.0.0-host-a.11`, SHA-256 `1f7528439bb0d1cc11041e81e3aed8849db66a0f19a8a38589058df2369d75c4`; not byte-identical to this candidate |
| Build workflow/run | `NOT EXERCISED` — local validation build, not a CI release build |
| Go toolchain | `go1.25.10 linux/arm64`, cross-built for Debian 13 amd64 |
| SBOM/provenance | `NOT EXERCISED` |
| Independent reviewer | `NOT EXERCISED` |

## HOST_B Identity

| Field | Recorded Value |
| --- | --- |
| Purpose | Docker coexistence, WAN gating, backend safety, and remaining DDNS variants |
| Image | Clean Debian GNU/Linux 13 (trixie); exact provider image build ID not retained |
| Architecture | `amd64` |
| Kernel | `6.12.95+deb13-amd64` |
| nftables | `1.1.3` |
| systemd | `257.13-1~deb13u1` |
| Docker client/server | `26.1.5+dfsg1` |
| Docker storage driver | `overlay2` |
| Validation UTC | 2026-07-13 approximately 13:03–13:28 |
| Console recovery | `NOT EXERCISED` — two remote reboots and reconnects passed |

The private host name, credentials, tester-controlled DNS names, public
addresses, and resolved allowlist values are intentionally omitted.

## Automated And Artifact Gate

| Check | Result | Evidence |
| --- | --- | --- |
| Formatting, tests, and vet | `PASS` | `sh ./scripts/check.sh` |
| Race tests | `PASS` | `go test -race ./...` |
| Focused Docker compatibility regressions | `PASS` | `go test ./internal/app ./internal/docker` |
| Delivery assets and systemd units | `PASS` | `scripts/verify-delivery-assets.sh` |
| Bundle lifecycle | `PASS` | Included in delivery-asset verification |
| Exact archive verification | `PASS` | Remote `verify-bundle` and `sha256sum -c SHA256SUMS` |
| Staticcheck/govulncheck | `NOT EXERCISED` | Tools unavailable locally |
| Shellcheck/actionlint | `NOT EXERCISED` | Tools unavailable locally |
| Repository sanitization | `PASS` | No validation host, credential, DNS name, or resolved address added to the repository |

## Docker And Coexistence Results

| Area | Result | Evidence/qualification |
| --- | --- | --- |
| Clean Docker baseline | `PASS` | Docker created only its own `ip nat` and `ip filter` tables before cnftctl installation |
| First install with Docker opt-in | `PASS` | Reconciliation active; firewall inactive until confirmed apply |
| Foreign nftables preservation | `PASS` | Separately owned `inet` table survived apply, confirm, rollback, firewall reload, and reboot when restored by its disposable boot owner before cnftctl loading |
| Docker table preservation | `PASS` | Docker tables survived apply/confirm/rollback and were recreated normally after both reboots |
| Default IPv4 WAN gate | `PASS` | Published TCP 18080 was reachable before cnftctl activation and timed out after activation with no matching open port |
| Matching IPv4 open | `PASS` | Confirmed TCP 18080 rule made the published service reachable from WAN |
| IPv4 close | `PASS` | Closing and confirming TCP 18080 blocked WAN access again without a Docker restart |
| IPv4 rollback | `PASS` | Unconfirmed open made the service reachable; explicit rollback restored the prior blocked policy |
| IPv4 original destination port | `PASS` | Container mapping used public 18080 to container 80; generated gate matched `ct original proto-dst` |
| Docker/container continuity | `PASS` | Container remained running with restart count 0 through policy changes; it returned normally after reboot under its test restart policy |
| Confirmed policy after reboot | `PASS` | Closed-port policy loaded at both boots and WAN TCP 18080 remained blocked |
| Docker IPv6 gate rendering | `PASS` | Exact active rules contained strict IPv6 DNAT and routed destination-port gates |
| Docker IPv6 WAN traffic | `NOT EXERCISED` | Validation runner had no IPv6 default route, so it could not originate an external IPv6 probe |

The disposable foreign table had no persistent owner during the first reboot
and therefore disappeared with kernel state, independently of cnftctl. For the
qualification reboot, a disposable owner recreated it before
`cnftctl-firewall.service`; cnftctl preserved it and Docker's tables.

## Docker Backend Safety

HOST_B found that Debian 13's Docker 26.1.5 rejects
`firewall-backend=nftables` as an unknown daemon option. The earlier candidate
could write that syntactically valid JSON, which would have prevented Docker
from starting after a later restart. No Docker restart occurred, the test file
was restored, and the daemon remained healthy.

The corrected `0.0.0-host-b.1` behavior was exercised against the installed
daemon:

| Area | Result | Evidence/qualification |
| --- | --- | --- |
| Plan is non-mutating | `PASS` | Exact daemon JSON checksum unchanged |
| Exact proposal compatibility validation | `PASS` | Live plan invoked Docker validation and refused the unsupported directive with exit 2 |
| Write without `--yes` | `PASS` | Refused with exit 2 and confirmation guidance |
| Authorized incompatible write | `PASS` | Refused before writing; error states that no file was written |
| Backup and daemon continuity on refusal | `PASS` | Backup count, daemon JSON checksum, Docker activation timestamp, and container restart count remained unchanged |
| Supported backend write | `NOT EXERCISED` | Installed Docker does not support the proposed backend; a real write would be unsafe and was correctly refused |

## DDNS Results

| Area | Result | Evidence/qualification |
| --- | --- | --- |
| A-only hostname | `PASS` | Exact IPv4 element added; absent AAAA family was not treated as total resolution failure |
| Dual-stack hostname `/56` | `PASS` | Exact A element and correctly masked native AAAA `/56` prefix populated with timeouts |
| Dual-stack hostname `/64` | `PASS` | Explicit `/64` selection produced the correct native AAAA `/64` prefix |
| Atomic all-host failure | `PASS` | Controlled resolver outage returned exit 2, preserved all previous live elements, and recorded `dns_temporary` metadata |
| Stale-after-full-TTL | `PASS` | With a temporary valid 30-second test TTL, elements expired and status returned exit 1 with `ddns_runtime_stale` after the full TTL |
| Normal TTL restoration | `PASS` | One-hour TTL, healthy runtime data, and active/desired agreement restored |
| Disable and rollback timer intent | `PASS` | Disable stopped and disabled the timer; rollback restored enabled and active state plus runtime elements |
| DDNS reboot intent | `PASS` | Timer remained enabled/active and runtime `/64` entries were healthy after reboot |

## Final Host State

At handoff, HOST_B is healthy on `0.0.0-host-b.1` with:

- no pending transaction;
- desired and active generation in agreement;
- Docker integration and the separately owned foreign table present;
- test container running;
- public TCP 18080 closed by cnftctl policy;
- dual-stack DDNS runtime healthy at `/64`; and
- DDNS refresh timer enabled and active.

The host is disposable and may be reimaged after any desired evidence export.

## Final Decision

- [x] Executed HOST_B Docker and DDNS checks passed for archive SHA-256 `dbb18f7a05636f4cc11d65fb229027d67ea2d01dedd436dc37ea81b6d5059620`.
- [x] All HOST_B failures and `NOT EXERCISED` items are disclosed.
- [ ] Docker IPv6 WAN traffic was exercised from an external IPv6-capable source.
- [ ] A clean HOST_A base-tier run passed for this exact archive SHA-256.
- [ ] Artifact identity matches a clean immutable commit, CI build, provenance, approval, and publication inputs.
- [ ] Independent reviewer approved promotion.

Decision: `REJECT FOR RELEASE PROMOTION — HOST_B COMPLETE; FINAL EXACT-ARTIFACT GATES PENDING`
