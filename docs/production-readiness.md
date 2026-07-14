# Production Readiness Status

This is the canonical gate for production-supported releases. A version is supported only after the exact `cnftctl_VERSION_amd64.deb` bytes pass every mandatory check below on Debian 13 amd64. Source checks and results from another delivery format are not substitutes. Published arm64 packages remain experimental until they independently complete the full gate described in `docs/manual-validation.md`.

## Current State

The native Debian package implementation, protected-branch workflow, automated
gate, and disposable-host HOST_A/HOST_B validation are complete. Historical
v0.1.0 evidence remains in `docs/validation-record-0.1.0.md`; the earlier tar
candidate is historical evidence only and was not published.

`v0.1.0` is published and supported. Technical validation, provider-KVM
recovery, build-once promotion, and public-download verification are complete.
The sanitized evidence and retained output index are recorded in release
evidence issue #8.

`v0.2.0` is published and production-supported on amd64. Its exact amd64
package completed the repeated gate on 2026-07-13; immutable identity, host
results, promotion run, and independent public-download verification are in
`docs/validation-record-0.2.0.md`. The released arm64 package remains
experimental and its live firewall checklist remains `NOT EXERCISED`.

Evidence may land after a build-once candidate and therefore advance `main`.
Each version tag must still point to its recorded candidate commit; promotion
permits `main` to be ahead while requiring exact tag/run commit identity.

## Mandatory Release Gate

Every artifact-dependent item must refer to the same package SHA-256. A package-byte or runtime-policy change creates a new candidate and restarts affected validation.

### 1. Review And Freeze Source

- [x] Merge the native-package implementation through a pull request.
- [x] Require `test`, `analysis`, `delivery-assets`, and `nft-syntax` on up-to-date PR branches.
- [x] Enforce PR-only `main` for administrators with zero required approvals, no force pushes, and no branch deletion.
- [x] Record clean formatting, unit tests, race tests, vet, staticcheck, govulncheck, actionlint, shellcheck, systemd validation, nft syntax, lintian, package reproducibility, licensing, and sanitization results.

### 2. Build And Identify Exact Package

- [x] Select immutable candidate commit `a2df7e38f77dba3b4dc236f7c3818c0b37749804` on `main` for `v0.2.0`.
- [x] Build `cnftctl_0.2.0_amd64.deb` and experimental `cnftctl_0.2.0_arm64.deb` exactly once in their native candidate jobs.
- [x] Record filename, byte size, SHA-256, commit, Go version, and build-run URL.
- [x] Verify control metadata, closed inventory, modes, manifest, installed checksums, maintainer scripts, and embedded CLI version offline.
- [x] Generate and retain architecture-matched SPDX SBOMs and keyless build provenance for both package byte streams.

### 3. Prepare Validation Host

- [x] Start with clean disposable Debian 13 amd64 without Docker for HOST_A.
- [x] Record image identity, kernel, nftables, systemd, architecture, and UTC timestamps.
- [x] Exercise provider-console or rescue recovery before firewall activation. The owner confirmed provider KVM login independent of SSH and nftables.
- [x] Keep a second SSH session and tested console path available throughout policy tests.

### 4. Validate Package And Base Lifecycle

- [x] Install the exact package with `apt` and verify it enables only reconciliation and activates no firewall policy or DDNS.
- [x] Verify Debian/version/architecture guards and package integrity reporting.
- [x] Verify `init --dry-run`, desired-config permissions/defaults, JSON reports, and exact candidate validation.
- [x] Verify first-install timeout removes only `inet hostfw`.
- [x] Verify confirmation persists and a later unconfirmed update restores the exact prior generation.
- [x] Verify generation files, manifests, modes, inventory, ownership, and hashes.

### 5. Validate Failure Recovery

- [x] Verify terminating the initiating SSH process does not cancel rollback.
- [x] Verify reboot reconciles every unconfirmed transaction to the last-known-good policy.
- [x] Verify confirmed policy loads through `cnftctl-firewall.service` after reboot.
- [x] Verify foreign nftables tables survive apply, confirm, timeout rollback, explicit rollback, and reboot when their owner provides boot recreation.
- [x] Capture journal ordering proving rollback supervision was active before selector and live-policy mutation.
- [x] Exercise incident inspection and recovery commands from provider-console access. Provider KVM supplies a root shell outside the nftables data path.

### 6. Validate SSH And DDNS

- [x] Verify uncovered-source refusal and audited lockout-risk override without any rollback bypass.
- [x] Verify initial DDNS seeding before hardened activation.
- [x] Verify A, AAAA `/56`, AAAA `/64`, timeouts, all-host failure, stale metadata, and atomic replacement.
- [x] Verify DDNS timer intent through apply, confirm, rollback, disable, and reboot.

### 7. Validate Docker Coexistence On HOST_B

- [x] Safely remove cnftctl after HOST_A, archive/purge disposable test state, verify `inet hostfw` is absent, and install Docker.
- [x] Reinstall the same package and verify Docker-owned tables survive every cnftctl lifecycle operation and reboot.
- [x] Verify Docker-to-host and Docker bridge behavior remains Docker-controlled when integration is enabled.
- [x] Verify published IPv4 traffic is blocked until matching `open_ports` intent is confirmed, closes again without a Docker restart, and uses the original public destination port.
- [x] Exercise IPv6 DNAT/routed gating when an external probe is available, otherwise record `NOT EXERCISED`.
- [x] Verify backend planning is non-mutating and unsupported writes are refused; a supported backend write remains deferred if Debian's Docker rejects it.

### 8. Validate Package Upgrade And Removal

- [x] Verify package upgrade preserves config, immutable generations, ownership, active policy, selection, and terminal audit history.
- [x] Verify pre-installation rejects unresolved, corrupt, malformed, duplicated-field, trailing-data, and symlinked transaction state, including after purge when the installed helper is absent.
- [x] Verify removal rejects active `inet hostfw` and unsafe transaction state.
- [x] Verify inactive `apt remove` and `apt purge` remove delivery assets, reload systemd, and preserve `/etc/cnftctl` and `/var/lib/cnftctl`.
- [x] Verify failures in required systemd operations abort safely without silently removing recovery coverage.

### 9. Record And Publish

- [x] Complete `docs/validation-record-0.2.0.md` and record the retained output index in release evidence issue #18. Sensitive infrastructure values remain excluded by policy.
- [x] Record every limitation and `NOT EXERCISED` item in release notes.
- [x] Self-review candidate identity, evidence, and publication inputs; no independent approval is required for this personal project.
- [x] Tag the exact validated source commit as `v0.2.0`.
- [x] Promote candidate run `29291333684` without rebuilding; promotion run `29294419952` passed.
- [x] Download the exact five public assets independently and repeat inventory, checksum, package-manifest, attestation, and native arm64 version verification.

## Stop Conditions

Do not publish when a mandatory check fails; artifact identity differs between CI, hosts, tag, and publication; evidence is missing or ambiguous; validation used rebuilt bytes or an unsupported host; or no tested out-of-band recovery route exists.

## Deferred Work

- Docker external IPv6 qualification and supported daemon-backend migration.
- Live Debian 13 arm64 qualification, other distributions, non-systemd systems, RPMs, and an APT repository.
- Independent long-lived signing keys beyond GitHub attestations and release checksums.
- SSH-disabled mode, fleet APIs, generic nftables management, and additional `openat2` state-path confinement.

## Source Documents

- Execution checklist: `docs/manual-validation.md`
- Fillable evidence record: `docs/validation-record.md`
- Release procedure: `docs/release-process.md`
- Current candidate release evidence: `docs/release-notes.md`
- Supported environments: `docs/support-matrix.md`
- Operational recovery: `docs/incident-response.md`
