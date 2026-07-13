# Release Process

Releases use SemVer tags `vMAJOR.MINOR.PATCH` and Conventional Commits by convention. Firewall semantics, config/preset compatibility, transaction behavior, and CLI automation contracts are compatibility surfaces. Before `v1.0.0`, any breaking change must still be explicit in release notes.

## Supported Artifact

Each release publishes `cnftctl_VERSION_amd64.deb` and experimental `cnftctl_VERSION_arm64.deb`. A standalone binary and the internal staging bundle are not supported installations because apply verifies package-installed systemd, recovery, manifest, and checksum assets.

Production support is not established until the exact amd64 package bytes complete the evidence gate below on Debian 13 amd64. Arm64 remains experimental until one exact released or candidate package independently completes the same full checklist on a suitable disposable Debian 13 arm64 host with console or rescue access.

The canonical remaining-work gate is `docs/production-readiness.md`. Record exact candidate and host results in `docs/validation-record.md`, using `docs/manual-validation.md` for executable steps.

## Required Evidence

The release issue and release body must identify:

- Tag, immutable commit SHA, both artifact filenames, byte sizes, and SHA-256 values.
- CI run URL for the tagged commit and successful formatting, tests, vet, package build, lintian, reproducibility, and staged-asset checks.
- Debian control metadata, installed manifest, checksum inventory, and successful offline `verify-deb.sh` output.
- Completed `docs/manual-validation.md` evidence from disposable Debian 13 amd64 hosts.
- nftables, systemd, kernel, and Docker versions exercised.
- First-install timeout deletion, prior-generation rollback, confirmation, session death, reboot reconciliation, DDNS, coexistence, Docker-table preservation, install, upgrade, and uninstall results.
- Dependency and Apache-2.0/third-party notice review.
- Known limitations, unsupported environments, and operator-impacting changes.
- Pull request and required-CI evidence for the candidate source commit.

Do not describe an unexecuted check as passed. Record unsupported or unexercised cases explicitly.

## Publication

1. Update release notes and support documentation.
2. Run `sh ./scripts/check.sh` and delivery-asset verification.
3. Build each architecture's Debian package exactly once in its native architecture job.
4. Verify both packages' metadata, manifests, ELF architecture, extracted contents, matching SBOMs, provenance, and checksums.
5. Complete exact-artifact manual validation.
6. Complete and self-review `docs/validation-record.md`; this personal project does not require an independent approver.
7. Create the SemVer tag only after the evidence record is complete.
8. Publish both unchanged packages, `sbom_amd64.spdx.json`, `sbom_arm64.spdx.json`, and deterministic `release-checksums.txt` together.
9. Download the public artifact in a clean environment and repeat package verification and installation.

Release automation must use least privilege, pin third-party actions to immutable commits, avoid `pull_request_target` for untrusted code, and expose no release secrets to pull-request jobs. Never force-push or silently replace a published artifact; issue a new version.

The build and promotion workflows are active together at `.github/workflows/release-build.yml` and `.github/workflows/release-promote.yml`. Native amd64 and arm64 jobs each build their package once, generate its architecture-named SBOM, and attest it; aggregation copies those unchanged bytes into a closed five-file candidate. Promotion remains an explicit manual action run from protected `main`. It explicitly checks out the requested release tag, requires the candidate run head SHA to equal that tag commit, verifies that the commit is on `main`, verifies the exact inventory and checksums, verifies both package manifests, and verifies both provenance attestations against the signer identity GitHub emits for the protected build workflow: `.github/workflows/release-build.yml@refs/heads/main`. The CLI verification also pins signer digest, source digest, and source ref to the tag commit and protected `main`. Changing that branch, path, or workflow name requires updating and validating the promotion identity checks before use.

`main` requires a pull request and the `test`, `analysis`, `delivery-assets`, and `nft-syntax` checks. No approving review is required, but administrators cannot bypass the PR or checks, and force pushes and deletion are disabled.

## Commit Convention

Use `<type>[optional scope]: <description>`, commonly `feat`, `fix`, `docs`, `test`, `ci`, `build`, `refactor`, or `chore`. Mark breaking changes with `!` and a `BREAKING CHANGE:` footer. Commit-message linting is not currently enforced.
