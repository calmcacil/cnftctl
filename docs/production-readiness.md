# Production Readiness Status

This document is the canonical list of work and evidence remaining before a production release. The implemented code has passed the repository's available unprivileged checks and repeated adversarial review, but **production status remains NOT READY** until the exact release archive passes the host validation gate below.

## Current State

Completed in the repository:

- Immutable, content-addressed firewall generations with exact-byte validation.
- Durable apply, confirm, rollback, and boot-reconciliation state transitions.
- Rollback supervision armed before policy activation.
- Targeted ownership of `inet hostfw` without modifying `/etc/nftables.conf` or foreign tables.
- Initial and runtime DDNS resolution with atomic dual-set replacement.
- SSH current-session coverage checks and audited lockout-risk override.
- Strict Docker WAN gating through `open_ports`.
- Versioned JSON health reports and conservative exit codes.
- Debian 13 amd64 bundle layout, systemd assets, installer, uninstaller, recovery helper, closed inventory, and checksums.
- Apache-2.0 licensing, security policy, support policy, operator guide, and incident procedures.
- Formatting, unit tests, race tests, vet, static analysis, vulnerability scanning, fault injection, fuzz smoke tests, bundle lifecycle tests, and workflow validation.

The immutable `0.1.0` candidate at commit
`88bbf3bb7847d82ea737a8aaa6ad73963f565b1b` was built by Release Candidate
Build run `29257634117`. Its exact archive SHA-256 is
`b77228ab67f19e3f484a9ce57f1fe3bd2ecfc546b4e1b888ac8e20ed4e810c0c`.
CI, provenance, SPDX attestation, offline verification, and the sanitized
HOST_A/HOST_B record at `docs/validation-record-0.1.0.md` are complete.

Not completed as release evidence:

- No final SemVer tag has been created.
- Provider-console recovery has not been exercised for this candidate.
- Raw host output and journals have not been attached to a release issue.
- No independent reviewer has approved the completed validation record.
- The protected `release` environment and independent approver still require repository-side configuration.
- No protected promotion or public-download reverification has been executed.

## Mandatory Release Gate

Every item below must be completed for the same archive SHA-256. A failure creates a new candidate and restarts all affected validation; do not replace bytes under an existing version.

### 1. Freeze The Candidate

- [x] Select a SemVer release candidate and immutable commit.
- [x] Confirm the worktree and module graph are clean.
- [x] Run `sh ./scripts/check.sh`.
- [x] Run `go test -count=1 ./...`.
- [x] Run `go test -race ./...`.
- [x] Run `staticcheck ./...` using the pinned release tool version.
- [x] Run `govulncheck ./...` and record reachable findings or approved exceptions.
- [x] Run `sh ./scripts/verify-delivery-assets.sh`.
- [x] Record actionlint, shellcheck, systemd-unit, staged nftables, bundle, license, notice, and sanitization results.

### 2. Build And Identify Exact Bytes

- [x] Build `cnftctl-VERSION-debian13-amd64.tar.gz` once from the selected commit.
- [x] Record archive filename, byte size, SHA-256, commit, Go version, and build-run URL.
- [x] Verify the extracted archive offline with `scripts/verify-bundle`.
- [x] Confirm the manifest says Debian 13, amd64, the intended version, and format version `1`.
- [x] Confirm every delivered regular file is covered by `SHA256SUMS` and no extra file or symlink exists.
- [x] Generate and retain the SPDX SBOM and keyless build provenance for these exact bytes.

### 3. Prepare Validation Hosts

- [x] Create clean disposable Debian 13 amd64 `HOST_A` without Docker.
- [x] Create clean disposable Debian 13 amd64 `HOST_B` with disposable Docker.
- [x] Record image identity, kernel, nftables, systemd, Docker, architecture, and UTC start time.
- [ ] Verify console or hypervisor recovery works before firewall activation.
- [ ] Keep an independent SSH session and console available throughout remote-policy tests.

### 4. Validate Installation And Base Lifecycle

- [x] Complete Artifact Identity and Staged Install sections in `docs/manual-validation.md`.
- [x] Verify installation activates no firewall policy and enables only boot reconciliation.
- [x] Verify `init --dry-run`, desired-config permissions/defaults, JSON reports, and exact candidate validation.
- [x] Verify first-install timeout removes only `inet hostfw`.
- [x] Verify confirmation persists and stops rollback supervision only after durable confirmation.
- [x] Verify a later unconfirmed update restores the exact previous generation.
- [x] Verify generation files, manifests, modes, inventory, and hashes.

### 5. Validate Failure Recovery

- [x] Verify terminating the initiating SSH process does not cancel rollback.
- [x] Verify reboot treats every unconfirmed transaction as failed and restores last-known-good policy.
- [x] Verify confirmed policy loads through `cnftctl-firewall.service` after reboot.
- [x] Verify foreign nftables tables survive apply, confirm, timeout rollback, explicit rollback, and reboot.
- [x] Capture timestamped journal evidence that rollback supervision was active before selector mutation and firewall activation.
- [ ] Exercise incident checks and recovery commands in `docs/incident-response.md` from console access.

### 6. Validate SSH And DDNS Safety

- [x] Verify an uncovered current SSH source blocks hardened apply.
- [x] Verify the lockout-risk override requires a reason and records it durably.
- [x] Verify the override does not bypass rollback or other readiness checks.
- [x] Verify initial DDNS entries are present in the activated exact generation before hardened policy takes effect.
- [x] Verify A entries, AAAA `/56`, AAAA `/64`, timeouts, all-host failure behavior, stale metadata, and one-batch replacement.
- [x] Verify DDNS timer enablement follows active-generation intent through apply, rollback, disable, and reboot.

### 7. Validate Docker Coexistence

- [x] Verify Docker-owned tables survive all cnftctl lifecycle operations and reboot.
- [x] Verify a published container port is blocked until matching `open_ports` intent is applied and confirmed.
- [x] Verify closing the port blocks it again without restarting Docker.
- [x] Verify IPv4 original public destination-port gating.
- [x] Validate IPv6 DNAT/routed behavior when the environment supports it, or record `NOT EXERCISED`; Docker remains experimental until separately qualified.
- [x] Verify live Docker daemon backend planning validates the exact proposal, is non-mutating, and refuses unsupported backends.
- [ ] On a daemon that supports the proposed backend, verify an authorized write preserves unrelated JSON, creates a backup, and does not restart Docker. This remains deferred with Docker production qualification because Debian Docker 26 rejects the option.

### 8. Validate Operations, Upgrade, And Uninstall

- [x] Verify healthy, degraded, pending, failed, unknown, absent, and unsupported report behavior where applicable.
- [x] Verify JSON schema `cnftctl.report.v1`, detail redaction, stdout purity, and exit codes `0`, `1`, and `2`.
- [x] Verify journals are actionable and contain no credentials or unsafe environment output.
- [x] Verify upgrade preserves config, immutable generations, ownership, active policy, and terminal transaction audit records.
- [x] Verify upgrade refuses unresolved, corrupt, malformed, or symlinked transaction state.
- [x] Verify uninstall refuses active policy and unresolved transactions.
- [x] Verify approved inactive uninstall removes delivery assets, reloads systemd, and preserves operator configuration unless separately purged.

### 9. Approve And Promote

- [x] Complete a version-specific copy of `docs/validation-record.md` without blank `PASS/FAIL/NOT EXERCISED` fields.
- [ ] Attach complete `docs/manual-validation.md` command output and journals to the release issue.
- [ ] Record known limitations and every `NOT EXERCISED` item in release notes.
- [ ] Obtain reviewer approval from someone other than the artifact producer.
- [x] Move both reviewed release workflows together to their documented active paths.
- [ ] Promote the exact validated bytes without rebuilding through the protected release environment.
- [ ] Download the public archive in a clean environment and repeat checksum, provenance, and bundle verification.

## Stop Conditions

Do not publish or deploy when any of these conditions exists:

- A critical or high code-review finding is unresolved.
- Any mandatory base-lifecycle, rollback, reboot, ownership, SSH, DDNS, installation, upgrade, or uninstall check fails.
- Artifact identity differs between CI, host validation, approval, and publication.
- Validation was performed on a source build, standalone binary, arm64 host, container, non-Debian-13 host, or rebuilt archive instead of the exact candidate.
- Required evidence is missing, ambiguous, edited to hide a failure, or recorded as passed without execution.
- The operator has no tested out-of-band recovery route.

## Deferred Work

These are not blockers for the narrow Debian 13 amd64 base release unless the release claims them as supported:

- Production qualification of Docker integration, especially IPv6 routing modes.
- Debian 13 arm64, Ubuntu, other distributions, and non-systemd systems.
- Native `.deb` or `.rpm` packages.
- Independent long-lived project signing keys.
- SSH-disabled mode.
- Docker-native publish-as-WAN-authority policy.
- Fleet APIs, daemons, metrics exporters, or generic nftables management.
- Linux `openat2` descriptor-relative state-path confinement as additional defense in depth.

## Source Documents

- Execution checklist: `docs/manual-validation.md`
- Fillable evidence record: `docs/validation-record.md`
- Release procedure: `docs/release-process.md`
- Release evidence template: `docs/release-notes.md`
- Supported and unsupported environments: `docs/support-matrix.md`
- Operational recovery: `docs/incident-response.md`
