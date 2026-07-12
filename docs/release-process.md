# Release Process

Releases use SemVer tags `vMAJOR.MINOR.PATCH` and Conventional Commits by convention. Firewall semantics, config/preset compatibility, transaction behavior, and CLI automation contracts are compatibility surfaces. Before `v1.0.0`, any breaking change must still be explicit in release notes.

## Supported Artifact

The canonical release artifact is exactly `cnftctl-VERSION-debian13-amd64.tar.gz`. A standalone binary is not a supported installation because apply verifies installed systemd and manifest assets.

**Production status is NOT READY** until this exact artifact completes the evidence gate below on Debian 13 amd64. The architecture contract and successful source checks are necessary but are not release evidence.

The canonical remaining-work gate is `docs/production-readiness.md`. Record exact candidate and host results in `docs/validation-record.md`, using `docs/manual-validation.md` for executable steps.

## Required Evidence

The release issue and release body must identify:

- Tag, immutable commit SHA, artifact filename, byte size, and SHA-256.
- CI run URL for the tagged commit and successful formatting, tests, vet, build, bundle, and staged-asset checks.
- Bundle manifest and successful offline `verify-bundle` output.
- Completed `docs/manual-validation.md` evidence from disposable Debian 13 amd64 hosts.
- nftables, systemd, kernel, and Docker versions exercised.
- First-install timeout deletion, prior-generation rollback, confirmation, session death, reboot reconciliation, DDNS, coexistence, Docker-table preservation, install, upgrade, and uninstall results.
- Dependency and Apache-2.0/third-party notice review.
- Known limitations, unsupported environments, and operator-impacting changes.
- Reviewer approval from someone other than the artifact producer.

Do not describe an unexecuted check as passed. Record unsupported or unexercised cases explicitly.

## Publication

1. Update release notes and support documentation.
2. Run `sh ./scripts/check.sh` and delivery-asset verification.
3. Build the bundle from the intended commit with the intended version.
4. Verify the archive after extraction and record its checksum.
5. Complete exact-artifact manual validation.
6. Complete `docs/validation-record.md` and obtain independent approval.
7. Create the SemVer tag only after evidence and approval are complete.
8. Publish the archive and checksum/provenance evidence together.
9. Download the public artifact in a clean environment and repeat bundle verification.

Release automation must use least privilege, pin third-party actions to immutable commits, avoid `pull_request_target` for untrusted code, and expose no release secrets to pull-request jobs. Never force-push or silently replace a published artifact; issue a new version.

The dormant workflows remain under `.github/workflows-disabled/` and must not be enabled piecemeal. Eventual activation must move them, without content or filename changes, to `.github/workflows/release-build.yml` and `.github/workflows/release-promote.yml`. The promotion workflow deliberately verifies attestations against the activated build path `actions/workflows/release-build.yml@$GITHUB_SHA`; changing that destination or the build workflow name requires updating and validating the promotion identity checks before activation.

## Commit Convention

Use `<type>[optional scope]: <description>`, commonly `feat`, `fix`, `docs`, `test`, `ci`, `build`, `refactor`, or `chore`. Mark breaking changes with `!` and a `BREAKING CHANGE:` footer. Commit-message linting is not currently enforced.
