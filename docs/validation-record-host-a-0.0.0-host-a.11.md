# HOST_A Validation Record: 0.0.0-host-a.11

This is the sanitized evidence record for the completed HOST_A run. It records
the exact locally built archive that was exercised. It is not release approval:
HOST_B, the unexercised DDNS cases, reproducible release provenance, and
independent review remain outstanding.

> Supersession note: HOST_B later found a Docker daemon compatibility defect.
> The corrected `0.0.0-host-b.1` archive has different bytes and must receive a
> clean exact-artifact HOST_A run before release promotion. This record remains
> valid evidence only for the SHA-256 recorded below.

## Candidate Identity

| Field | Recorded Value |
| --- | --- |
| Version/tag | `0.0.0-host-a.11` (validation version; no release tag) |
| Source identity | Dirty validation workspace based on commit `fc51df5612cb10605c13fca2f970dd1a9b4be10b`; the archive includes the uncommitted fixes exercised during HOST_A validation |
| Artifact filename | `cnftctl-0.0.0-host-a.11-debian13-amd64.tar.gz` |
| Artifact byte size | `2142364` |
| Artifact SHA-256 | `1f7528439bb0d1cc11041e81e3aed8849db66a0f19a8a38589058df2369d75c4` |
| Build workflow/run | `NOT EXERCISED` — local validation build, not a CI release build |
| Go toolchain | `go1.25.10 linux/arm64`, cross-built for Debian 13 amd64 |
| SBOM identity | `NOT EXERCISED` |
| Provenance identity | `NOT EXERCISED` |
| Artifact producer | Local validation workspace |
| Independent reviewer | `NOT EXERCISED` |

## Host Identity

| Field | HOST_A | HOST_B |
| --- | --- | --- |
| Purpose | Base firewall lifecycle | Docker coexistence |
| Image/build ID | Clean Debian 13 image; exact build ID not retained | `NOT EXERCISED` |
| Architecture | `amd64` | `NOT EXERCISED` |
| Kernel | Version output not retained | `NOT EXERCISED` |
| nftables | Debian 13 nftables `1.1.3` | `NOT EXERCISED` |
| systemd | Version output not retained | `NOT EXERCISED` |
| Docker | Not installed | `NOT EXERCISED` |
| Console recovery tested | `NOT EXERCISED` — remote reboot and reconciliation passed; no console recovery was required | `NOT EXERCISED` |
| Validation start/end UTC | Exact boundaries not retained | `NOT EXERCISED` |

The private host name, credentials, tester-controlled DDNS name, and resolved
addresses are intentionally omitted.

## Automated Gate

| Check | Result | Evidence |
| --- | --- | --- |
| Formatting, tests, and vet | `PASS` | `sh ./scripts/check.sh` |
| Uncached tests | `PASS` | Full package tests were rerun after the final fixes |
| Race tests | `PASS` | `go test -race ./...` |
| Staticcheck | `NOT EXERCISED` | Tool unavailable locally |
| govulncheck | `NOT EXERCISED` | Tool unavailable locally |
| Debian amd64 build | `PASS` | CLI cross-build completed |
| Bundle lifecycle tests | `PASS` | Bundle install/uninstall lifecycle suite |
| systemd unit validation | `PASS` | Delivery assets and units were verified and exercised on HOST_A |
| Staged nftables syntax | `PASS` | Exact generated configuration passed real Debian 13 `nft -c` validation |
| Shellcheck/actionlint | `NOT EXERCISED` | Tools unavailable locally |
| License/notices/sanitization | `PASS` | Delivery-asset verification and repository sanitization search passed; no private HOST_A or DDNS values were added |
| Offline bundle verification | `PASS` | `scripts/verify-delivery-assets.sh` and extracted bundle verification passed |

## HOST_A Validation Results

| Area | Result | Evidence/qualification |
| --- | --- | --- |
| Staged install/uninstall contract | `PASS` | Exact bundle staged lifecycle completed |
| First live install activates no policy | `PASS` | Install/init/validate/plan left `inet hostfw` absent |
| Desired config defaults and permissions | `PASS` | Defaults and installed permissions checked |
| Exact candidate validation | `PASS` | Candidate bytes validated with generation-relative includes and nft include path |
| First-install timeout removes only `inet hostfw` | `PASS` | Fresh apply timed out to the safe absent-table state |
| Confirmed generation persists | `PASS` | Confirmation remained active beyond the rollback deadline |
| Update timeout restores exact prior generation | `PASS` | Exact prior-generation ruleset restored |
| Initiating-session death rollback | `PASS` | Rollback completed after the applying SSH session ended |
| Reboot reconciliation | `PASS` | Unconfirmed state reconciled after remote reboot |
| Confirmed policy boot loading | `PASS` | Selected generation loaded at boot |
| SSH uncovered-source refusal | `PASS` | Unsafe hardened apply refused |
| SSH override reason and audit | `PASS` | Missing reason refused; explicit reason persisted; rollback remained armed |
| DDNS initial candidate seeding | `PASS` | A-only hostname seeded exact IPv4 elements |
| DDNS A records | `PASS` | Refresh and timeout-element status parsing exercised with nftables 1.1.3 |
| DDNS AAAA `/56` | `NOT EXERCISED` | Tester-controlled hostname had no AAAA record |
| DDNS AAAA `/64` | `NOT EXERCISED` | Tester-controlled hostname had no AAAA record |
| DDNS all-host failure and stale state | `NOT EXERCISED` | A-only missing-AAAA handling passed; stale-after-full-TTL behavior was not run |
| DDNS timer apply/rollback intent | `PASS` | Timer enablement, disablement, and rollback restoration passed |
| DDNS timer reboot intent | `NOT EXERCISED` | DDNS timer intent was not separately verified across reboot |
| Foreign nftables table preservation | `PASS` | Pre-existing tables survived managed-table activation and targeted cleanup |
| Docker table preservation | `NOT EXERCISED` | Reserved for HOST_B |
| Docker IPv4 WAN gate | `NOT EXERCISED` | Reserved for HOST_B |
| Docker IPv6 DNAT/routed gate | `NOT EXERCISED` | Reserved for HOST_B |
| JSON schema, redaction, and exit codes | `PASS` | Healthy `0`, unhealthy inspection `1`, and command failure `2` exercised |
| Journal content and ordering | `PASS` | Unit and transaction outcomes were inspected during rollback, reboot, DDNS, and reconciliation tests; raw journal export was not retained |
| Upgrade with terminal audit history | `PASS` | Desired config, generations, history, and active policy survived upgrade |
| Upgrade refusal for unsafe transaction state | `NOT EXERCISED` | No corrupt/unsafe durable transaction fixture was run on HOST_A |
| Active-policy uninstall refusal | `PASS` | Uninstall refused while the managed table was active |
| Approved inactive uninstall | `PASS` | After targeted `nft delete table inet hostfw`, uninstall succeeded and preserved config, generations, and history |
| Post-uninstall staged validation | `PASS` | Revalidated using the exact bundle binary |

HOST_A ended uninstalled with `inet hostfw` absent and operator configuration,
generations, and transaction history preserved. The host may now be reimaged for
HOST_B without losing any unrecorded HOST_A test requirement; the limitations
above remain release-gate work, not HOST_A rerun requirements.

## Ordering Evidence

The live lifecycle tests observed the required sequence: durable transaction
creation, verified rollback arming, activation, confirmation-before-cleanup, or
safe rollback to the prior generation/absent-table state. Reboot reconciliation
and initiating-session loss were also exercised. Raw journal output was not
retained, so a release candidate should retain timestamped logs as publication
evidence.

## Defects Found And Corrected During HOST_A

- Debian `VERSION_CODENAME=trixie` parsing rejected literal `t` as though it
  were a tab escape.
- Absolute generation includes broke exact candidate nft validation.
- Empty open-port and whitelist rendering produced invalid nftables syntax.
- Normal SSH applies incorrectly recorded override metadata.
- The firewall unit used the wrong Debian nft binary path.
- Reconciliation reran during apply and was not reliably pulled into boot.
- Boot reconciliation mishandled a safely absent managed table.
- Sandboxed services lacked the required runtime directory.
- A-only DDNS resolution treated the missing AAAA family as a total failure.
- nftables 1.1.3 timeout elements used a nested JSON representation not handled
  by status parsing.
- Generated initial DDNS elements incorrectly caused permanent desired/active
  drift.

Each correction was included in the archive identified above and covered by a
focused regression test before the final full validation pass.

## Known Limitations And Remaining Gates

| Item | Status/Rationale |
| --- | --- |
| HOST_A base lifecycle | `COMPLETE` for the exact archive SHA-256 above, subject to the disclosed unexercised variants |
| Docker production qualification | `NOT EXERCISED` — HOST_B required |
| Docker IPv6 | `NOT EXERCISED` — HOST_B capability-dependent |
| Native DDNS IPv6 prefix behavior | `NOT EXERCISED` — `/56` and `/64` require a controlled AAAA record |
| DDNS stale-after-full-TTL behavior | `NOT EXERCISED` |
| Experimental platforms | Unsupported; Debian 13 amd64 remains the only production target |
| Vulnerability scan | `NOT EXERCISED` — govulncheck unavailable locally |
| Release provenance/SBOM | `NOT EXERCISED` |
| Independent review | `NOT EXERCISED` |

## Final Decision

- [x] Executed HOST_A base-tier checks passed for archive SHA-256 `1f7528439bb0d1cc11041e81e3aed8849db66a0f19a8a38589058df2369d75c4`.
- [x] HOST_A failures and `NOT EXERCISED` items are disclosed in this record.
- [ ] HOST_B Docker coexistence checks passed for the same candidate source.
- [ ] Artifact identity matches a clean immutable commit, CI build, provenance, approval, and publication inputs.
- [ ] Independent reviewer checked the evidence and approved promotion.
- [ ] Publicly downloaded bytes were reverified after publication.

Decision: `REJECT FOR RELEASE PROMOTION — HOST_A COMPLETE; REMAINING GATES PENDING`

Rationale: the HOST_A run is complete and the host can be reimaged for HOST_B.
This local dirty-workspace validation artifact is not itself eligible for
release promotion.
