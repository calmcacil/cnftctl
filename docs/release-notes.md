# cnftctl First Release Notes

This file is the release note and checklist template for the first `cnftctl` release.

## Release Summary

`cnftctl` manages a specific nftables firewall profile for Linux hosts. The first release focuses on preserving the sanitized reference firewall behavior while adding a safer CLI-oriented workflow for configuration, validation, planning, and active-policy application.

The project remains intentionally narrow. It is not a universal firewall manager.

## Included Scope

- Go module: `github.com/calmcacil/cnftctl`.
- Managed firewall profile: `inet hostfw`.
- Baseline reference behavior from `reference/nftables.conf`.
- Public WAN open-port model through `open_ports`.
- SSH safety model with open-by-default behavior and explicit hardening.
- Optional DDNS SSH whitelist behavior with `/56` default IPv6 prefix derivation and `/64` support.
- Optional Docker WAN gating using the same open-port policy.
- Sanitized examples for config and presets.
- CI for formatting, tests, vet/static analysis, and CLI builds.
- Disabled future release binary scaffolding kept outside active GitHub Actions workflows.

## Security Highlights

- Active firewall policy changes must use a dead-man rollback flow.
- `cnftctl confirm` must be required before the rollback timeout expires.
- Presets are untrusted input and must not bypass validation or confirmation.
- Docker daemon edits and restarts must require explicit consent.
- Examples must not contain real domains, tokens, private addresses, or personal allowlists.
- The binary name is `cnftctl`; do not publish an executable named `nftctl`.

## Known Limitations To Confirm Before Publishing

- Confirm `./cmd/cnftctl` builds for the release commit.
- Confirm the exact implemented command list before copying command examples into the GitHub release body.
- Confirm the DDNS implementation strategy: retained POSIX updater or Go-native refresh.
- Confirm Docker nftables backend behavior on target Docker Engine versions.
- Confirm target distro nftables syntax compatibility.

## Release Checklist

- Run `go test ./...`.
- Run `go vet ./...`.
- Run `test -z "$(gofmt -l .)"`.
- Run `go build -o ./bin/cnftctl ./cmd/cnftctl`.
- Run the checklist in `docs/manual-validation.md` on a disposable or recoverable host.
- Review `README.md`, `SPEC.md`, and examples for current command names and behavior.
- Verify no examples or docs contain real secrets or personal infrastructure values.
- Create a signed or protected release tag only after release publishing is explicitly enabled.
- Confirm the disabled release workflow has been reviewed before moving it into active workflows.
- Confirm the future release workflow builds Linux binaries and `checksums.txt` before enabling publication.
- Attach manual validation notes to the release.

## Suggested GitHub Release Body

```markdown
## cnftctl first release

This release introduces the first packaged `cnftctl` workflow for managing the repository's nftables firewall profile.

### Safety

Firewall changes can lock you out of a remote host. Use console/rescue access and validate rollback before applying on a production machine.

### Install

Download the Linux binary for your architecture, verify `checksums.txt`, and install it as `/usr/local/bin/cnftctl`.

### Validate

Run `cnftctl init --dry-run`, review `cnftctl plan`, apply with the dead-man rollback flow, and confirm only after verifying access.

### Notes

- Docker integration is opt-in.
- DDNS hostnames are part of the SSH trust boundary.
- Presets are untrusted input and must be reviewed before use.
```
