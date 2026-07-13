# Release Validation Record: 0.1.0 Debian Candidate

This record covers the native Debian package proposed for `v0.1.0`. It is
technical release evidence, not proof of the post-publication download check.
The tag, promotion, and public-download checks have not run. The owner has
confirmed provider KVM login independent of SSH and nftables.

HOST_A and HOST_B were consecutive phases on one disposable Debian 13 amd64
VPS. The host was returned to an inactive state after validation: `cnftctl`
was purged, `inet hostfw` and the disposable foreign table were absent, the
test container was removed, Docker remained installed, and the documented
operator configuration and audit state remained preserved.

## Candidate Identity

| Field | Recorded Value |
| --- | --- |
| Version/tag | `0.1.0` candidate; tag not yet created |
| Commit SHA | `ee7ab0fd6932bafe1c22b684ec72e27e50803f94` |
| Artifact filename | `cnftctl_0.1.0_amd64.deb` |
| Artifact byte size | `1712440` |
| Artifact SHA-256 | `93966559a326522a984cc8dcd36a062d5f4931a8c51cacecdc847664b277b198` |
| Build workflow/run | [Release Candidate Build 29282503578](https://github.com/calmcacil/cnftctl/actions/runs/29282503578) |
| Go toolchain | `go1.26.5`; static Linux amd64 binary |
| SBOM | `sbom.spdx.json`, SHA-256 `6ec93d2fd915890ef76880a917dd4b46f73a8eb7d1b4cb16434d45ded4637be0` |
| Checksums file | `release-checksums.txt`, SHA-256 `927764deed126f8dfa6955ec3fa63c2a8aadd7feb669359b6d2bb033cb413a40` |
| Provenance | GitHub API returned two attestations for the exact package digest: build provenance and SPDX 2.3 SBOM |
| Workflow artifact | `cnftctl-0.1.0-candidate`, artifact ID `8291920144`, container digest `sha256:116bcb4be4e0d0a51c4ae34be4890a9d6453f7bdfd019df8abc78b00f9c8fe8b` |
| Artifact producer | GitHub-hosted Release Candidate Build workflow |
| Candidate self-review | `PASS`; source changes merged through PRs #4, #5, and #6 with protected CI |

The artifact contained exactly the package, checksums file, and SBOM.
`sha256sum --check`, `verify-deb.sh`, closed inventory, control metadata,
installed checksums, modes, amd64 identity, embedded version, lintian, and
reproducible package tests passed.

The final candidate supersedes run `29269726273`. A package-content comparison
showed that the replacement changes the incoming `preinst` validator, package
metadata/checksums, changelog timestamp, and Go build VCS identity. Runtime
source and firewall policy are unchanged. The full HOST_A/HOST_B run completed
before that packaging-only correction; all affected install, upgrade, removal,
reinstall, rollback, and Docker checks were then repeated with the final bytes.

## Host Identity

| Field | HOST_A | HOST_B |
| --- | --- | --- |
| Purpose | Base lifecycle, SSH, DDNS, rollback, reboot, upgrade/removal | Docker coexistence, WAN gating, backend refusal, final package lifecycle |
| Image | Fresh Debian GNU/Linux 13 (trixie) | Same disposable host after HOST_A state archive/purge and Docker installation |
| Architecture | `amd64` | `amd64` |
| Kernel | `6.12.95+deb13-amd64` | `6.12.95+deb13-amd64` |
| nftables | `1.1.3` | `1.1.3` |
| systemd | `257.13-1~deb13u1` | `257.13-1~deb13u1` |
| Docker | Not installed | `26.1.5+dfsg1-9+b13` |
| Validation UTC | 2026-07-13 17:20–19:25 | 2026-07-13 19:36–20:36 |
| Console recovery | `PASS`; owner confirmed provider KVM login independent of SSH/nftables | `PASS`; the same provider KVM path remains available |

Private host names, credentials, public addresses, tester-controlled DDNS
names, and resolved allowlist values are intentionally omitted.

## Automated Gate

| Check | Result | Evidence |
| --- | --- | --- |
| Formatting, tests, vet, and build | `PASS` | PR #6 CI and candidate run `29282503578` |
| Uncached tests | `PASS` | Candidate run `29282503578` |
| Race tests | `PASS` | Candidate run `29282503578` |
| Staticcheck | `PASS` | `v0.8.0-rc.1`, all checks with documented exclusions |
| govulncheck | `PASS` | `v1.6.0`; no vulnerabilities found |
| Package reproducibility and lifecycle | `PASS` | CI, candidate workflow, offline download, and live host |
| Lintian | `PASS` | `--fail-on error`; no warnings for the final package |
| systemd unit validation | `PASS` | Delivery checks and live systemd |
| Staged nftables syntax | `PASS` | Debian 13 nftables validation plus live activation |
| ShellCheck and actionlint | `PASS` | PR #6 CI and candidate run |
| License, notices, and sanitization | `PASS` | Closed inventory and repository checks |
| Offline package verification | `PASS` | Exact downloaded SHA-256 and `verify-deb.sh` |
| SBOM and provenance | `PASS` | Exact SBOM hash and two package attestations |

## HOST_A Results

| Area | Result | Evidence/qualification |
| --- | --- | --- |
| First live install activation contract | `PASS` | Reconciliation enabled; firewall disabled, DDNS disabled, and `inet hostfw` absent |
| Dry-run, defaults, permissions, and JSON | `PASS` | Dry-run wrote nothing; config `0600`; report schema and exit codes validated |
| First-install timeout | `PASS` | Verified rollback timer removed only `inet hostfw` and wrote terminal audit state |
| Confirm and prior-generation rollback | `PASS` | Confirmation survived deadline; later timeout and explicit rollback restored the exact prior generation |
| Immutable generation integrity | `PASS` | Manifests, file modes, inventory, selector, ownership, and hashes validated |
| Initiating-session death | `PASS` | Independent systemd timer completed rollback after the invoking SSH process ended |
| Boot reconciliation | `PASS` | Immediate reboot reconciled the unconfirmed transaction to the confirmed generation |
| Foreign-table ownership | `PASS` | Apply, confirm, rollback, and reboot never flushed unrelated nftables ownership |
| Hardened SSH refusal | `PASS` | Uncovered current source refused before activation |
| SSH override audit | `PASS` | Missing reason refused; explicit reason was durable; rollback remained mandatory |
| DDNS initial seeding | `PASS` | Resolved entries existed before hardened activation |
| DDNS A and AAAA `/56` | `PASS` | Exact A element and correctly masked `/56` prefix with timeouts |
| DDNS AAAA `/64` | `PASS` | Explicit `/64` selection produced the exact prefix |
| DDNS failure, freshness, and timer | `PASS` | Atomic all-host failure, stale state, refresh, timer intent, disable, rollback, and reboot exercised |
| Upgrade and reinstall | `PASS` | Configuration, generations, selector, policy, and terminal audit history preserved |
| Unsafe package operations | `PASS` | Pending, corrupt, duplicate-field, trailing-data, and symlinked state refused |
| Removal contract | `PASS` | Active removal refused; approved inactive remove and purge preserved `/etc/cnftctl` and `/var/lib/cnftctl` |

## HOST_B Results

| Area | Result | Evidence/qualification |
| --- | --- | --- |
| Docker baseline | `PASS` | Docker owned `ip nat` and `ip filter`; two published ports were reachable before cnftctl gating |
| Default IPv4 WAN gate | `PASS` | Both published ports timed out with empty matching `open_ports` intent |
| Original public destination port | `PASS` | Public 18080 and 18082 mapped to container port 80; each required its own public-port tuple |
| Matching open and close | `PASS` | Confirmed open returned HTTP 200; confirmed close timed out without Docker restart |
| Docker internal traffic | `PASS` | Docker-to-host SSH and host-to-container published service remained available |
| Rollback | `PASS` | Explicit rollback restored the prior closed policy while Docker tables and container identity remained |
| Reboot coexistence | `PASS` | cnftctl loaded first; Docker then recreated its tables without conflict. The test container correctly stayed stopped because its Docker restart policy was `no` |
| Foreign ownership | `PASS` | Foreign table survived every live cnftctl operation; its lack of a boot owner was correctly not attributed to cnftctl |
| Backend plan/write | `PASS` | Debian Docker 26 rejected unsupported `firewall-backend`; plan, missing `--yes`, and authorized write paths made no file or container change |
| Exact final active reinstall | `PASS` | Package integrity, config, generation, audit, selection, policy, Docker start time, and foreign table were unchanged |
| Exact final purge/reinstall | `PASS` | Purge preserved state; malformed preserved state refused before unpack; clean preserved state installed successfully without an installed helper |
| Exact final Docker gate | `PASS` | Apply, confirm, rollback, open, close, external probe, and internal connectivity repeated with SHA-256 `93966559...b277b198` |
| Docker IPv6 external traffic | `NOT EXERCISED` | Generated strict IPv6 DNAT/routed rules passed nftables validation; no external IPv6 probe was required for this release |
| Docker-aware final cleanup | `PASS` | Targeted deactivation/purge left Docker tables installed, removed only test objects, and preserved operator/audit state |

## Ordering Evidence

Transaction state and systemd journals showed this order for live applies:

1. Durable transaction prepared.
2. Rollback service and timer armed and verified active.
3. Transaction marked rollback-armed, then activating.
4. Active selector and ownership updated.
5. `cnftctl-firewall.service` activated exact immutable bytes.
6. Transaction marked activated.
7. Confirmation persisted before timer cleanup, or rollback restored the prior generation/table state.

No secret, credential, or DDNS trust value appeared in retained journals.

## Known Limitations And Remaining Gates

| Item | Status/Rationale |
| --- | --- |
| Provider-console/rescue recovery | `PASS`; owner confirmed provider KVM login. Providers without KVM/console/rescue necessarily rely on the independently supervised rollback system |
| Docker IPv6 external probe | `NOT EXERCISED`; exact generated rules and nftables syntax passed; IPv4 is the qualified external path |
| Supported Docker backend write | `NOT EXERCISED`; Debian Docker 26 rejects the directive, and writing an invalid daemon configuration would be unsafe |
| Evidence issue | [Issue #8](https://github.com/calmcacil/cnftctl/issues/8) records candidate identity, retained output index, automated raw-log locations, KVM confirmation, and sanitization exclusions |
| Final tag and promotion | Not run; `v0.1.0` must point to candidate commit `ee7ab0f...`, even though this evidence PR advances `main`, and promotion must use run `29282503578` without rebuilding |
| Public download verification | Requires publication and remains outstanding |
| Vulnerability exceptions | None |

## Final Decision

- [x] All source, package, HOST_A, and HOST_B technical checks passed, with scoped revalidation of the packaging-only final change.
- [x] Artifact identity matches the immutable commit, build run, downloaded bytes, SBOM, attestations, and host package.
- [x] Every unexercised case and limitation is disclosed.
- [x] Candidate identity and evidence received a self-review; independent approval is not required.
- [x] Provider KVM recovery has been exercised and recorded.
- [x] Sanitized evidence and the retained output index are recorded in release evidence issue #8.
- [ ] The exact source commit has been tagged and candidate run promoted without rebuilding.
- [ ] Publicly downloaded release bytes have been reverified.

Decision: `APPROVE FOR TAG AND PROMOTION — POST-PUBLICATION DOWNLOAD VERIFICATION REMAINS MANDATORY`
