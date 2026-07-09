# Release Process

This repository uses SemVer for versioning and Conventional Commits for commit-message consistency. Release publishing is not enabled yet.

## Current Automation

- Active CI lives in `.github/workflows/ci.yml` and runs on pull requests, pushes to `main`, and manual dispatch.
- CI runs formatting, tests, vet, and a CLI build with repository-local commands only.
- Future release-build scaffolding lives in `.github/workflows-disabled/release-build.yml` and is intentionally not executable by GitHub Actions.

## SemVer Policy

Use tags in the form `vMAJOR.MINOR.PATCH` when releases are enabled.

- Increment `MAJOR` for incompatible CLI, config, preset, or firewall behavior changes.
- Increment `MINOR` for backward-compatible features, commands, config options, or release artifact additions.
- Increment `PATCH` for bug fixes, documentation corrections, validation improvements, and non-breaking test or build changes.

Before `v1.0.0`, minor versions may still include breaking changes, but release notes must call out those changes clearly. Be conservative when versioning changes that affect firewall behavior, SSH safety, rollback behavior, or config compatibility.

Do not create or push release tags until release publishing is explicitly approved.

## Conventional Commits

Use this format:

```text
<type>[optional scope]: <description>
```

Common types:

- `feat`: user-visible feature or behavior addition.
- `fix`: bug fix.
- `docs`: documentation-only change.
- `test`: test-only change.
- `ci`: CI or workflow change.
- `build`: build, packaging, or dependency-management change.
- `refactor`: internal restructuring without behavior change.
- `chore`: maintenance not covered by other types.

Breaking changes should use `!` after the type or scope and include a `BREAKING CHANGE:` footer.

Examples:

```text
ci: add Go validation workflow
feat(config): add DDNS IPv6 prefix option
fix(apply): reject concurrent transactions
feat(config)!: rename SSH mode field

BREAKING CHANGE: configs using ssh.mode must migrate to ssh.access_mode.
```

Commit-message linting is not enforced yet. If enforcement is added later, it must avoid `pull_request_target` and must not expose secrets to untrusted pull request code.

## Disabled Release Build

The disabled release scaffold is a draft for future Linux binary packaging. It is kept outside `.github/workflows/` so GitHub Actions will not run it.

Before enabling release automation:

- Confirm `docs/manual-validation.md` passes on a disposable or recoverable host.
- Confirm artifact contents contain only intentional sanitized files.
- Confirm `checksums.txt` generation and publication expectations.
- Confirm tag protection or signing policy.
- Confirm whether binary signing is required.
- Confirm GitHub Release publishing permissions.
- Review workflow permissions before granting any write access.

Until those items are complete, do not publish GitHub Releases, upload public release assets, create or push tags, publish packages, or require repository secrets for releases.
