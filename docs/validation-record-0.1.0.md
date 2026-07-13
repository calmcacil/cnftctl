# Release Validation Record: 0.1.0 Candidate

This is the sanitized validation record for the immutable GitHub Actions
candidate identified below. HOST_A and HOST_B were two consecutive clean
phases on the same disposable Debian 13 amd64 VPS. After HOST_A, the approved
targeted deactivation and bundle uninstall completed, preserved state was
archived, all cnftctl state was purged, and absence of `inet hostfw` was
verified before Docker was installed for HOST_B.

This record is release evidence, not release approval. Independent review,
the protected release environment, the final tag, promotion, and public
download verification remain outstanding.

## Candidate Identity

| Field | Recorded Value |
| --- | --- |
| Version/tag | `0.1.0` candidate; no tag created |
| Commit SHA | `88bbf3bb7847d82ea737a8aaa6ad73963f565b1b` |
| Artifact filename | `cnftctl-0.1.0-debian13-amd64.tar.gz` |
| Artifact byte size | `2180733` |
| Artifact SHA-256 | `b77228ab67f19e3f484a9ce57f1fe3bd2ecfc546b4e1b888ac8e20ed4e810c0c` |
| Build workflow/run | Release Candidate Build run `29257634117` |
| CI workflow/run | CI run `29257633227` |
| Go toolchain | `go1.26.5`; static Linux amd64 delivery binary |
| SBOM | `sbom.spdx.json`, SHA-256 `43a82ebcc57dd3fba8e1bdd3148b74fabba05c71ca26a576bf74f0eacad144c9` |
| Attestations | GitHub API returned one SLSA provenance and one SPDX 2.3 attestation for the archive digest |
| Artifact producer | GitHub-hosted Release Candidate Build workflow |
| Independent reviewer | `NOT EXERCISED` |

The downloaded candidate contained exactly the archive, checksum file, and
SBOM. `sha256sum --check`, offline extracted-bundle verification, manifest
inventory, amd64 ELF identity, and versioned manifest checks passed.

## Host Identity

| Field | HOST_A | HOST_B |
| --- | --- | --- |
| Purpose | Base lifecycle, SSH, DDNS, upgrade, uninstall | Docker coexistence and WAN gating |
| Image | Fresh Debian GNU/Linux 13 (trixie) | Same disposable host after verified cnftctl uninstall and state purge |
| Architecture | `amd64` | `amd64` |
| Kernel | `6.12.95+deb13-amd64` | `6.12.95+deb13-amd64` |
| nftables | `1.1.3` | `1.1.3` |
| systemd | `257.13-1~deb13u1` | `257.13-1~deb13u1` |
| Docker | Not installed | Client/server `26.1.5+dfsg1` |
| Validation UTC | 2026-07-13 14:31–14:51 | 2026-07-13 14:52–15:08 |
| Console recovery | `NOT EXERCISED`; remote reboot/reconnect passed | `NOT EXERCISED`; remote reboot/reconnect passed |

The private host name, credentials, public addresses, tester-controlled DDNS
name, and resolved allowlist values are intentionally omitted. Raw HOST_A and
HOST_B evidence archives are retained outside the repository with SHA-256
`d339c2b70ef1fb9c12fe03ca984aaf91770f1988c70ce939d8f075c572308123`
and `ed73031238908ac504dd2c2f298a16f7ed0473945237022a6077b267d5438bd8`
respectively.

## Automated Gate

| Check | Result | Evidence |
| --- | --- | --- |
| Formatting, tests, vet, and build | `PASS` | CI and candidate build workflows |
| Uncached and race tests | `PASS` | Release Candidate Build run `29257634117` |
| Staticcheck | `PASS` | Pinned `2025.1.1` |
| govulncheck | `PASS` | Pinned `v1.1.4`; no reachable vulnerabilities |
| Fuzz smoke and fault boundaries | `PASS` | All five fuzz targets and apply fault-boundary test rerun under Go 1.26.5 |
| Bundle lifecycle and offline verification | `PASS` | CI, candidate workflow, local download, HOST_A, and HOST_B |
| systemd units and staged nftables | `PASS` | Delivery checks plus real Debian nftables validation |
| Shellcheck and actionlint | `PASS` | CI and candidate workflow |
| License, notices, and sanitization | `PASS` | Closed bundle inventory and repository review |
| Provenance and SBOM | `PASS` | Two attestations associated with the exact archive digest |

## HOST_A Results

| Area | Result | Evidence/qualification |
| --- | --- | --- |
| Staged install/uninstall | `PASS` | Exact bundle installed and removed from an alternate root |
| First live install activation contract | `PASS` | Reconciliation enabled; firewall disabled and `inet hostfw` absent |
| Dry-run, defaults, permissions, and JSON | `PASS` | Dry-run wrote nothing; config `0600`; schema `cnftctl.report.v1` |
| First-install timeout | `PASS` | Timer armed; timeout removed only `inet hostfw`; terminal state recorded `fresh_install` and `rolled-back` |
| Confirmed generation | `PASS` | Timer stopped and generation persisted beyond its deadline |
| Prior-generation timeout rollback | `PASS` | Active selector and exact prior nftables output restored |
| Immutable generation integrity | `PASS` | Directory mode `0500`; manifest identity, modes, sizes, and hashes validated |
| Initiating-session death | `PASS` | Applying SSH session ended; independent timer restored the prior generation |
| Boot reconciliation | `PASS` | Immediate reboot rolled back the unconfirmed transaction before loading the confirmed generation |
| Foreign-table ownership | `PASS` | Targeted operations never used `flush ruleset`; foreign ownership was preserved where present |
| Hardened SSH refusal | `PASS` | Uncovered current source refused before activation |
| SSH override audit | `PASS` | Missing reason refused; explicit reason/context persisted; rollback remained armed |
| DDNS initial seeding | `PASS` | Initial resolved entries existed before confirmed activation |
| DDNS A and native AAAA `/56` | `PASS` | Exact A element and correctly masked `/56` prefix with timeouts |
| DDNS native AAAA `/64` | `PASS` | Explicit `/64` selection produced the correct prefix |
| DDNS atomic failure and staleness | `PASS` | All-host failure returned exit 2 without partial replacement; full test TTL expired and status returned stale exit 1 |
| DDNS timer intent | `PASS` | Disable stopped timer; rollback restored it; confirmed disable survived reboot |
| Status, doctor, and exit codes | `PASS` | Healthy `0`, degraded inspection `1`, and command error `2`; JSON parsed |
| Journals and ordering | `PASS` | Timer was verified active before apply returned; rollback, reconciliation, firewall, and DDNS journals retained without credential output |
| Upgrade | `PASS` | Prior HOST_A bundle to 0.1.0 preserved config, selector, audit history, and exact live table bytes |
| Unsafe upgrade/uninstall refusal | `PASS` | Pending and deliberately malformed transaction state both refused |
| Active-policy uninstall refusal | `PASS` | Delivery removal refused while `inet hostfw` was active |
| Approved inactive uninstall | `PASS` | Targeted table deletion followed by uninstall removed delivery assets and preserved operator state |

The first timeout exposed only an observation race in the test command: table
removal became visible immediately before the terminal state-file replacement.
The settled state was valid and `transactions list` reported no pending
transaction. No unsafe or ambiguous durable state was observed.

## HOST_B Results

| Area | Result | Evidence/qualification |
| --- | --- | --- |
| Clean Docker baseline | `PASS` | Docker owned `ip nat` and `ip filter`; published TCP 18080 was reachable before cnftctl |
| Docker opt-in install | `PASS` | Install remained inactive; explicit Docker-enabled init/apply/confirm required |
| Default IPv4 WAN gate | `PASS` | Published TCP 18080 timed out without matching open-port intent |
| Matching open and close | `PASS` | Confirmed open allowed WAN access; confirmed close blocked it without Docker restart |
| Original destination-port gate | `PASS` | Public 18080 opened while public 18081, DNATed to the same container port, remained blocked |
| Rollback | `PASS` | Explicit and timer-expiry rollback restored the closed policy and kept Docker and foreign tables plus both containers intact |
| Docker and foreign tables | `PASS` | Tables survived apply, confirm, rollback, targeted uninstall, and boot recreation by their owners |
| Reboot | `PASS` | Confirmed closed policy, Docker containers/tables, and separately owned table returned; WAN remained blocked |
| Container continuity | `PASS` | Container identities and restart counts did not change during policy operations or backend planning |
| Backend status | `PASS` | Installed daemon reported no explicit firewall backend |
| Backend plan | `PASS` | Docker 26 validated and rejected unsupported `firewall-backend`; exact input JSON was unchanged |
| Backend write confirmation | `PASS` | Missing `--yes` refused without mutation or restart |
| Authorized incompatible write | `PASS` | Exact proposal failed daemon validation before write; no backup, restart, or container change occurred |
| Supported backend write | `NOT EXERCISED` | Debian Docker 26 does not support this directive; writing it would be unsafe |
| Docker IPv6 rule behavior | `PASS` | Exact generated rules contain strict routed and DNAT destination-port gates |
| Docker IPv6 external traffic | `NOT EXERCISED` | Accepted limitation: no external IPv6 probe was run |
| Docker-aware uninstall | `PASS` | After targeted removal, Docker/foreign tables and containers remained; published service became reachable again |

## Known Limitations And Remaining Gates

| Item | Status/Rationale |
| --- | --- |
| Docker IPv6 external probe | `NOT EXERCISED`; exact rules were validated and Docker remains experimental |
| Supported daemon backend write | `NOT EXERCISED`; installed Docker correctly rejects the option |
| Provider console recovery | `NOT EXERCISED`; remote reboot/reconciliation passed, but an out-of-band recovery drill remains required by the release gate |
| Independent review | Required before tagging or promotion |
| Branch protection | Repository ruleset not yet configured |
| Protected release environment | Not yet configured; must require an independent reviewer |
| Final tag and promotion | Not created or run |
| Public download verification | Requires publication and remains outstanding |

## Final Decision

- [x] All executable HOST_A base-tier checks passed for archive SHA-256 `b77228ab67f19e3f484a9ce57f1fe3bd2ecfc546b4e1b888ac8e20ed4e810c0c`.
- [x] HOST_B Docker checks passed for the same archive, with external IPv6 and a supported backend write explicitly unexercised.
- [x] Artifact identity matches the immutable commit, CI candidate, downloaded bytes, SBOM, and attestations.
- [ ] Provider console recovery has been exercised and recorded.
- [ ] An independent reviewer has approved this evidence.
- [ ] The exact candidate has been tagged and promoted through the protected environment.
- [ ] Publicly downloaded release bytes have been reverified.

Decision: `REJECT FOR RELEASE PROMOTION — TECHNICAL VALIDATION COMPLETE; EXTERNAL GOVERNANCE AND CONSOLE GATES PENDING`
