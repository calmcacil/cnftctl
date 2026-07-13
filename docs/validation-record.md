# Release Validation Record

Copy this file into the release issue or a version-specific evidence document. Do not edit this template to imply that checks have run. Use `PASS`, `FAIL`, or `NOT EXERCISED`, attach raw output, and explain every failure or unexercised item.

## Candidate Identity

| Field | Recorded Value |
| --- | --- |
| Version/tag | `RECORD_AT_VALIDATION` |
| Commit SHA | `RECORD_AT_VALIDATION` |
| amd64 artifact filename | `cnftctl_VERSION_amd64.deb` |
| amd64 artifact byte size | `RECORD_AT_VALIDATION` |
| amd64 artifact SHA-256 | `RECORD_AT_VALIDATION` |
| arm64 artifact filename | `cnftctl_VERSION_arm64.deb` |
| arm64 artifact byte size | `RECORD_AT_VALIDATION` |
| arm64 artifact SHA-256 | `RECORD_AT_VALIDATION` |
| Build workflow/run | `RECORD_AT_VALIDATION` |
| Go toolchain | `RECORD_AT_VALIDATION` |
| SBOM identities | `sbom_amd64.spdx.json` / `sbom_arm64.spdx.json`; `RECORD_AT_VALIDATION` |
| Provenance identities | `RECORD_AT_VALIDATION` |
| Artifact producer | `RECORD_AT_VALIDATION` |
| Candidate self-review | `RECORD_AT_VALIDATION` |

## Host Identity

| Field | HOST_A | HOST_B |
| --- | --- | --- |
| Purpose | Base firewall lifecycle | Docker coexistence |
| Image/build ID | `RECORD` | `RECORD` |
| Architecture | `amd64` | `amd64` |
| Kernel | `RECORD` | `RECORD` |
| nftables | `RECORD` | `RECORD` |
| systemd | `RECORD` | `RECORD` |
| Docker | `not installed` | `RECORD` |
| Console recovery tested | `PASS/FAIL` | `PASS/FAIL` |
| Validation start/end UTC | `RECORD` | `RECORD` |

## Automated Gate

| Check | Result | Evidence URL/Attachment |
| --- | --- | --- |
| Formatting, tests, and vet | `PASS/FAIL` | `RECORD` |
| Uncached tests | `PASS/FAIL` | `RECORD` |
| Race tests | `PASS/FAIL` | `RECORD` |
| Staticcheck | `PASS/FAIL` | `RECORD` |
| govulncheck | `PASS/FAIL` | `RECORD` |
| Package reproducibility and lifecycle tests | `PASS/FAIL` | `RECORD` |
| Lintian | `PASS/FAIL` | `RECORD` |
| systemd unit validation | `PASS/FAIL` | `RECORD` |
| Staged nftables syntax | `PASS/FAIL` | `RECORD` |
| Shellcheck/actionlint | `PASS/FAIL` | `RECORD` |
| License/notices/sanitization | `PASS/FAIL` | `RECORD` |
| Offline package verification | `PASS/FAIL` | `RECORD` |

## Host Validation Results

| Area | Result | Evidence URL/Attachment |
| --- | --- | --- |
| Package install/remove contract | `PASS/FAIL` | `RECORD` |
| First live install activates no policy | `PASS/FAIL` | `RECORD` |
| Desired config defaults and permissions | `PASS/FAIL` | `RECORD` |
| Exact candidate validation | `PASS/FAIL` | `RECORD` |
| First-install timeout removes only `inet hostfw` | `PASS/FAIL` | `RECORD` |
| Confirmed generation persists | `PASS/FAIL` | `RECORD` |
| Update timeout restores exact prior generation | `PASS/FAIL` | `RECORD` |
| Initiating-session death rollback | `PASS/FAIL` | `RECORD` |
| Reboot reconciliation | `PASS/FAIL` | `RECORD` |
| Confirmed policy boot loading | `PASS/FAIL` | `RECORD` |
| SSH uncovered-source refusal | `PASS/FAIL` | `RECORD` |
| SSH override reason and audit | `PASS/FAIL` | `RECORD` |
| DDNS initial candidate seeding | `PASS/FAIL` | `RECORD` |
| DDNS A and AAAA `/56` | `PASS/FAIL` | `RECORD` |
| DDNS AAAA `/64` | `PASS/FAIL` | `RECORD` |
| DDNS all-host failure and stale state | `PASS/FAIL` | `RECORD` |
| DDNS timer apply/rollback/reboot intent | `PASS/FAIL` | `RECORD` |
| Foreign nftables table preservation | `PASS/FAIL` | `RECORD` |
| Docker table preservation | `PASS/FAIL/NOT EXERCISED` | `RECORD` |
| Docker IPv4 WAN gate | `PASS/FAIL/NOT EXERCISED` | `RECORD` |
| Docker IPv6 DNAT/routed gate | `PASS/FAIL/NOT EXERCISED` | `RECORD` |
| JSON schema, redaction, and exit codes | `PASS/FAIL` | `RECORD` |
| Journal content and ordering | `PASS/FAIL` | `RECORD` |
| Upgrade with terminal audit history | `PASS/FAIL` | `RECORD` |
| Upgrade refusal for unsafe transaction state | `PASS/FAIL` | `RECORD` |
| Active-policy uninstall refusal | `PASS/FAIL` | `RECORD` |
| Approved inactive uninstall | `PASS/FAIL` | `RECORD` |

## Ordering Evidence

Record UTC journal timestamps or attach logs proving this order:

1. Durable transaction prepared.
2. Rollback service and timer armed.
3. Timer verified active.
4. Transaction marked rollback-armed.
5. Transaction marked activating before selector or live-policy mutation.
6. Active selector and ownership updated.
7. Managed firewall service activated.
8. Transaction marked activated.
9. Confirmation persisted before timer cleanup, or rollback restored safe state.

Evidence: `RECORD_AT_VALIDATION`

## Known Limitations And Exceptions

| Item | Status/Rationale |
| --- | --- |
| Docker production qualification | `RECORD` |
| Docker IPv6 | `RECORD` |
| Experimental platforms | `RECORD` |
| Vulnerability exceptions | `RECORD OR NONE` |
| Other unexercised behavior | `RECORD OR NONE` |

## Experimental arm64 Validation

Until an exact arm64 package completes the full checklist on a disposable
Debian 13 arm64 host with independent console or rescue access, record every
live item as `NOT EXERCISED`, never `PASS`. Native CI, package lifecycle tests,
and successful execution on a CI runner are compatibility evidence only.

Arm64 live result: `NOT EXERCISED / PASS / FAIL`

Evidence and rationale: `RECORD`

## Final Decision

- [ ] Every mandatory non-Docker base-tier check passed for the same package SHA-256.
- [ ] All failures and `NOT EXERCISED` items are disclosed in release notes.
- [ ] Artifact identity matches build, host evidence, self-review, and publication inputs.
- [ ] Candidate evidence and known limitations received an explicit self-review.
- [ ] Publicly downloaded bytes were reverified after publication.

Decision: `APPROVE / REJECT`

Owner, UTC timestamp, and rationale: `RECORD_AT_DECISION`
