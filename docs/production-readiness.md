# Production Readiness Status

This is the canonical gate for the first supported release. A version is supported only after the exact `cnftctl_VERSION_amd64.deb` bytes pass every mandatory check below on Debian 13 amd64. Source checks and results from another delivery format are not substitutes.

## Current State

The firewall engine, rollback lifecycle, SSH safety, DDNS behavior, Docker WAN gate, reporting, and systemd integration have completed prior source and host validation. The earlier `0.1.0` tar candidate is retained as historical engineering evidence in `docs/validation-record-0.1.0-tar-candidate.md`, but it is superseded and will not be published.

The public delivery contract is now a native Debian package. Its implementation, CI candidate, clean-host validation, provider-console drill, protected-branch workflow, tag, promotion, and public-download verification must complete before `v0.1.0`.

## Mandatory Release Gate

Every artifact-dependent item must refer to the same package SHA-256. A package-byte or runtime-policy change creates a new candidate and restarts affected validation.

### 1. Review And Freeze Source

- [ ] Merge the native-package implementation through a pull request.
- [ ] Require `test`, `analysis`, `delivery-assets`, and `nft-syntax` on up-to-date PR branches.
- [ ] Enforce PR-only `main` for administrators with zero required approvals, no force pushes, and no branch deletion.
- [ ] Record clean formatting, unit tests, race tests, vet, staticcheck, govulncheck, actionlint, shellcheck, systemd validation, nft syntax, lintian, package reproducibility, licensing, and sanitization results.

### 2. Build And Identify Exact Package

- [ ] Select the immutable `v0.1.0` source commit on `main`.
- [ ] Build `cnftctl_0.1.0_amd64.deb` once in the candidate workflow.
- [ ] Record filename, byte size, SHA-256, commit, Go version, and build-run URL.
- [ ] Verify control metadata, closed inventory, modes, manifest, installed checksums, maintainer scripts, and embedded CLI version offline.
- [ ] Generate and retain the SPDX SBOM and keyless build provenance for the package bytes.

### 3. Prepare Validation Host

- [ ] Start with clean disposable Debian 13 amd64 without Docker for HOST_A.
- [ ] Record image identity, kernel, nftables, systemd, architecture, and UTC timestamps.
- [ ] Exercise provider-console or rescue recovery before firewall activation.
- [ ] Keep a second SSH session and tested console path available throughout policy tests.

### 4. Validate Package And Base Lifecycle

- [ ] Install the exact package with `apt` and verify it enables only reconciliation and activates no firewall policy or DDNS.
- [ ] Verify Debian/version/architecture guards and package integrity reporting.
- [ ] Verify `init --dry-run`, desired-config permissions/defaults, JSON reports, and exact candidate validation.
- [ ] Verify first-install timeout removes only `inet hostfw`.
- [ ] Verify confirmation persists and a later unconfirmed update restores the exact prior generation.
- [ ] Verify generation files, manifests, modes, inventory, ownership, and hashes.

### 5. Validate Failure Recovery

- [ ] Verify terminating the initiating SSH process does not cancel rollback.
- [ ] Verify reboot reconciles every unconfirmed transaction to the last-known-good policy.
- [ ] Verify confirmed policy loads through `cnftctl-firewall.service` after reboot.
- [ ] Verify foreign nftables tables survive apply, confirm, timeout rollback, explicit rollback, and reboot.
- [ ] Capture journal ordering proving rollback supervision was active before selector and live-policy mutation.
- [ ] Exercise incident inspection and recovery commands from provider-console access.

### 6. Validate SSH And DDNS

- [ ] Verify uncovered-source refusal and audited lockout-risk override without any rollback bypass.
- [ ] Verify initial DDNS seeding before hardened activation.
- [ ] Verify A, AAAA `/56`, AAAA `/64`, timeouts, all-host failure, stale metadata, and atomic replacement.
- [ ] Verify DDNS timer intent through apply, confirm, rollback, disable, and reboot.

### 7. Validate Docker Coexistence On HOST_B

- [ ] Safely remove cnftctl after HOST_A, archive/purge disposable test state, verify `inet hostfw` is absent, and install Docker.
- [ ] Reinstall the same package and verify Docker-owned tables survive every cnftctl lifecycle operation and reboot.
- [ ] Verify Docker-to-host and Docker bridge behavior remains Docker-controlled when integration is enabled.
- [ ] Verify published IPv4 traffic is blocked until matching `open_ports` intent is confirmed, closes again without a Docker restart, and uses the original public destination port.
- [ ] Exercise IPv6 DNAT/routed gating when an external probe is available, otherwise record `NOT EXERCISED`.
- [ ] Verify backend planning is non-mutating and unsupported writes are refused; a supported backend write remains deferred if Debian's Docker rejects it.

### 8. Validate Package Upgrade And Removal

- [ ] Verify package upgrade preserves config, immutable generations, ownership, active policy, selection, and terminal audit history.
- [ ] Verify pre-installation rejects unresolved, corrupt, malformed, duplicated-field, trailing-data, and symlinked transaction state.
- [ ] Verify removal rejects active `inet hostfw` and unsafe transaction state.
- [ ] Verify inactive `apt remove` and `apt purge` remove delivery assets, reload systemd, and preserve `/etc/cnftctl` and `/var/lib/cnftctl`.
- [ ] Verify failures in required systemd operations abort safely without silently removing recovery coverage.

### 9. Record And Publish

- [ ] Complete `docs/validation-record-0.1.0.md` with raw command output and journals attached to the release issue.
- [ ] Record every limitation and `NOT EXERCISED` item in release notes.
- [ ] Self-review candidate identity, evidence, and publication inputs; no independent approval is required for this personal project.
- [ ] Tag the exact validated source commit as `v0.1.0`.
- [ ] Promote the candidate package without rebuilding.
- [ ] Download the public package in a clean Debian 13 environment and repeat checksum, provenance, package, installation, version, and removal verification.

## Stop Conditions

Do not publish when a mandatory check fails; artifact identity differs between CI, hosts, tag, and publication; evidence is missing or ambiguous; validation used rebuilt bytes or an unsupported host; or no tested out-of-band recovery route exists.

## Deferred Work

- Docker external IPv6 qualification and supported daemon-backend migration.
- Debian 13 arm64, other distributions, non-systemd systems, RPMs, and an APT repository.
- Independent long-lived signing keys beyond GitHub attestations and release checksums.
- SSH-disabled mode, fleet APIs, generic nftables management, and additional `openat2` state-path confinement.

## Source Documents

- Execution checklist: `docs/manual-validation.md`
- Fillable evidence record: `docs/validation-record.md`
- Release procedure: `docs/release-process.md`
- Release evidence template: `docs/release-notes.md`
- Supported environments: `docs/support-matrix.md`
- Operational recovery: `docs/incident-response.md`
