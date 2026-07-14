# AGENTS.md

This file is the operating contract for humans and coding agents working on
cnftctl. Follow it together with `SPEC.md`; when code, documentation, and this
file disagree, investigate the current implementation and active workflows
before changing behavior or making support claims.

## Mission And Scope

- The Go module is `github.com/calmcacil/cnftctl`. The CLI entrypoint is
  `cmd/cnftctl/main.go`; application behavior is primarily under `internal/*`.
- cnftctl is intentionally narrow. It owns only the nftables table
  `inet hostfw`; it is not a general nftables editor, Docker firewall owner,
  router policy manager, or fleet control plane.
- `SPEC.md` is the implemented architecture and invariant record.
  `reference/` is a sanitized legacy behavior baseline, not a supported
  installation or runtime architecture.
- The supported delivery unit is the published Debian package. Standalone
  binaries and the internal staging bundle are development artifacts because
  live activation depends on package-installed systemd units, recovery tools,
  manifests, and checksums.
- The project was created primarily for personal use. Tests and evidence are
  best-effort risk controls, not a guarantee. Documentation must keep operator
  responsibility, trust review, and recovery planning explicit.

## Support Policy

- Production support is exactly Debian 13 (`trixie`) `amd64`, with Debian's
  systemd and nftables, and only for exact released package bytes that completed
  the full release evidence gate.
- Debian 13 `arm64` packages are published but experimental, unvalidated on a
  disposable live host, not production/security-supported, and used at the
  operator's own risk.
- Native arm64 CI proves build/package compatibility; it does not replace live
  activation, rollback, reboot/recovery, DDNS, Docker coexistence, upgrade, and
  uninstall validation with independent console/rescue access.
- Arm64 graduates only when one exact released or candidate arm64 package
  completes every mandatory item in `docs/manual-validation.md` on a suitable
  disposable host and its identity/evidence is recorded.
- Do not broaden distribution, architecture, Docker, network-mode, or security
  claims from compilation, unit tests, staged syntax checks, or a different
  package. Report unexecuted live checks as `NOT EXERCISED`, never `PASS`.
- Architecture-neutral policy rendering, nftables behavior, transaction logic,
  rollback, and systemd units should remain shared. In particular,
  `SystemCallArchitectures=native` is correct for both packaged architectures.

## Non-Negotiable Firewall Safety Invariants

- Never use or recommend `flush ruleset`. Cleanup and emergency deactivation
  may target only `table inet hostfw`; Docker and other owners may have tables
  in the same ruleset.
- Mutable `/etc/cnftctl/config.yaml` is desired operator intent. Active policy
  comes only from immutable content-addressed generations under
  `/var/lib/cnftctl/generations/`. Never load mutable desired files as active
  policy or edit generation contents in place.
- Every non-test live policy change must validate the exact final bytes, write
  a durable generation and transaction, arm and verify systemd-owned rollback
  before selector/live mutation, activate through
  `cnftctl-firewall.service`, and require `cnftctl confirm`.
- The 120-second rollback deadline and boot reconciliation are safety features.
  Do not add, document, or imply a production bypass. Dry-run and alternate-root
  test paths must remain clearly non-production.
- A first-install rollback deletes only `inet hostfw`; a later rollback restores
  the recorded prior generation. Reboot must reconcile every unconfirmed
  durable transaction.
- SSH remains open from WAN by default. `whitelist-only` and
  `whitelist-rate-limit` are explicit hardening choices with lockout warnings.
  Preserve current-session coverage checks and the audited acknowledgement plus
  non-empty reason. The acknowledgement may accept risk but must never disable
  validation or rollback.
- `open_ports` entries are public WAN exposure for matching host services and,
  when Docker gating is enabled, matching Docker-published protocol/public-port
  tuples. Explain both consequences anywhere exposure is configured.
- Docker integration is opt-in and must not take ownership of Docker's tables
  or bridge behavior. Editing `daemon.json` and restarting Docker are separate,
  disruptive actions requiring explicit operator consent. cnftctl must validate
  the exact proposed daemon JSON, preserve unrelated keys, back it up, refuse
  unsupported backends without mutation, and never restart Docker itself.
- Trusted interfaces are explicit full host-input trust. Never auto-discover
  them or confuse overlay identity/ACLs with cnftctl policy.
- DDNS names are part of the SSH trust boundary. A records are exact IPv4
  entries; AAAA entries derive `/56` prefixes by default, with `/64` only for
  intentional single-LAN trust. Preserve atomic replacement, timeouts,
  freshness reporting, and active-generation-derived timer state.
- Presets are untrusted input. They may pre-fill desired config but cannot
  bypass schema validation, human risk explanation, local write confirmation,
  SSH coverage, apply, or rollback.
- Removal must refuse active `inet hostfw` and unresolved, corrupt, or unsafe
  transaction history. `apt remove` and `apt purge` preserve `/etc/cnftctl` and
  `/var/lib/cnftctl`; destructive retained-state cleanup is separate and
  explicit.

## Working Method

1. Read the relevant sections of `SPEC.md`, operator documentation, active
   workflows, and tests before modifying behavior. Do not infer current state
   from an old plan, issue, or historical validation record.
2. Inspect the existing worktree and preserve unrelated user changes and
   untracked files. Never use destructive reset/checkout commands to make the
   tree convenient.
3. For code discovery, prefer the configured codebase knowledge graph
   (`search_graph`, `trace_path`, `get_code_snippet`, then `query_graph` or
   `get_architecture`) when available. Use `rg` for literals, shell/config/docs,
   or when graph results are insufficient.
4. Keep changes narrow and testable. Do not opportunistically redesign firewall
   semantics, expand ownership, weaken guards, or mix release evidence with
   later feature/documentation work.
5. Add or update tests for behavior changes. Treat error paths, no-op/idempotent
   behavior, state corruption, permissions, symlinks, exact bytes, and rollback
   ordering as first-class cases.
6. Update `SPEC.md` when implemented architecture or invariants change. Update
   the operator guide, command/install/uninstall docs, support matrix, release
   process, manual checklist, security policy, and examples when their contracts
   are affected.
7. Use SemVer and Conventional Commit messages by convention. Before `v1.0.0`,
   breaking CLI, config, preset, transaction, or automation changes still need
   explicit release-note treatment.

## Commands And Verification

- CI-equivalent Go checks:

  ```sh
  sh ./scripts/check.sh
  ```

  This runs `gofmt -l .`, `go test ./...`, and `go vet ./...`.

- Build the development CLI:

  ```sh
  go build -o ./bin/cnftctl ./cmd/cnftctl
  ```

- Run focused tests with normal Go package selection, for example:

  ```sh
  go test ./internal/app -run TestName
  go test ./internal/render
  ```

- Update render snapshots only for an intentional reviewed renderer change:

  ```sh
  UPDATE_SNAPSHOTS=1 go test ./internal/render
  ```

  Snapshot whitespace is part of the exact generated output.

- Delivery interfaces require explicit architecture and accept only `amd64` or
  `arm64`:

  ```sh
  sh ./scripts/build-bundle.sh VERSION ARCH OUTPUT_DIRECTORY
  sh ./scripts/build-deb.sh VERSION ARCH OUTPUT.deb
  sh ./scripts/verify-deb.sh PACKAGE VERSION ARCH
  ```

- Shell scripts must remain POSIX `sh` compatible and dependency-light. Run
  delivery lifecycle, reproducibility, shellcheck, lintian, actionlint,
  systemd-unit, and staged nftables checks in proportion to affected code.
- The reference nftables config contains absolute `/etc/nftables.d/*.nft`
  includes. Validate it from a staged `/etc` layout or a disposable host with
  `nft -c -f /etc/nftables.conf`, never directly from the checkout path.
- Do not update snapshots or expected package inventories merely to make an
  unexplained failure pass. Establish why bytes changed and review the safety
  impact first.

## CI, Pull Requests, And Supply Chain

- Files under `.github/workflows/` are the authority for active automation.
  Current documentation must match them; historical workflow behavior is not
  evidence of the current gate.
- `main` is PR-only and protected by `test`, `analysis`, `delivery-assets`, and
  `nft-syntax`. Native delivery jobs run amd64 on `ubuntu-latest` and arm64 on
  GitHub-hosted `ubuntu-24.04-arm`.
- Workflow permissions must be least-privilege. Pin third-party actions to
  immutable commit SHAs. Never expose secrets to untrusted PR code or introduce
  secret-bearing `pull_request_target` execution.
- Package tests on each native runner must cover Go tests, lifecycle behavior,
  reproducibility, package verification, native binary execution/version, and
  lintian. Debian 13 staged nftables syntax remains architecture-independent
  coverage.
- Keep generated package inventories closed: expected files, permissions,
  control architecture, installed delivery manifest architecture, ELF
  architecture, embedded version, and checksums must all be verified.
- Preserve the user's authorship and unrelated work. Do not force-push shared
  branches, bypass protected checks, or replace published artifacts.

## Build-Once Release Method

1. Merge source through protected CI and select one immutable commit on `main`.
2. Dispatch the candidate workflow with the intended SemVer version.
3. Build each package exactly once in its matching native job:
   `cnftctl_VERSION_amd64.deb` and `cnftctl_VERSION_arm64.deb`.
4. Generate `sbom_amd64.spdx.json` and `sbom_arm64.spdx.json`; attest each
   package with its matching SBOM and protected workflow/source identity.
5. Aggregate unchanged bytes into one candidate containing exactly those four
   files plus deterministic `release-checksums.txt` covering the four inputs.
6. Complete the full manual exact-package checklist on disposable Debian 13
   amd64 with independent console/rescue access. Keep arm64 live items
   `NOT EXERCISED` until its separate graduation gate is actually performed.
7. Record candidate commit, run, filenames, sizes, hashes, environment,
   exercises, limitations, and retained evidence. A byte change creates a new
   candidate and invalidates artifact-dependent evidence.
8. Land sanitized evidence, then create the release tag at the exact candidate
   commit even if documentation has advanced `main` afterward.
9. Dispatch promotion with the exact version and candidate run ID. Promotion
   must verify tag/run commit equality, ancestry, successful workflow identity,
   exact five-file inventory, deterministic checksums, both manifests, and both
   attestations including signer workflow, signer digest, source digest, and
   source ref.
10. Publish both packages, both SBOMs, and the checksum file without rebuilding.
    Download the public assets independently, reverify inventory, bytes,
    packages, native execution where available, and attestations, then record
    and close release evidence.

Never publish when identity is ambiguous, a mandatory check failed, required
evidence is missing, a rebuilt artifact was substituted, or the validation host
lacked a tested recovery path. Never silently replace a release asset; issue a
new version.

## Documentation And Evidence Discipline

- User-facing entry points are `README.md`, `docs/install.md`,
  `docs/uninstall.md`, `docs/commands.md`, and `docs/operator-guide.md`.
  Safety-critical instructions must agree across them.
- `docs/release-notes.md` describes the current release. Historical release
  notes and validation records are immutable evidence: do not rewrite old
  results to match newer methods or support claims.
- `docs/manual-validation.md` is the executable exact-artifact checklist;
  `docs/validation-record.md` is its blank evidence template;
  versioned validation records contain performed results.
- Keep `PASS`, `FAIL`, and `NOT EXERCISED` literal and honest. Separate CI
  compatibility evidence from disposable-host production evidence.
- Examples must use reserved documentation values such as `example.com`,
  `203.0.113.0/24`, `198.51.100.0/24`, and `2001:db8::/32`.
- Never commit credentials, Cloudflare tokens, private keys, real hostnames,
  private infrastructure addresses, personal allowlists, raw sensitive logs,
  SSH connection context, or operator secrets. Sanitize evidence before review;
  retain sensitive raw evidence privately only when required.

## Security And Dependency Changes

- Follow `SECURITY.md` for vulnerability handling; do not put undisclosed
  vulnerabilities or real infrastructure details in public issues.
- Treat changes to nftables rendering, activation order, rollback, transaction
  parsing, path handling, package maintainer scripts, systemd units, Docker
  integration, presets, CI permissions, action pins, and release identity as
  security-sensitive.
- Preserve Apache-2.0 licensing, `THIRD_PARTY_NOTICES.md`, dependency review,
  SBOM generation, and vulnerability/static analysis. Do not add copied code or
  dependencies without compatible licensing and a concrete need.
- The server DDNS updater intentionally uses `nft getent awk sort systemd`.
  The EdgeRouter updater at `reference/cloudflare-ddns.sh` intentionally uses
  `sh curl ip awk logger` without `jq` or Python and is not a server-side script.
