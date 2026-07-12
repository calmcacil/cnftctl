# AGENTS.md

## Project Shape
- This is the Go module `github.com/calmcacil/cnftctl`; CLI entrypoint is `cmd/cnftctl/main.go`, and most behavior is in `internal/*` packages.
- `SPEC.md` records the approved implemented architecture. `reference/` remains a sanitized firewall behavior baseline, but it is not the supported install/runtime architecture.
- `cnftctl` is intentionally narrow: it manages the app-owned `inet hostfw` nftables profile, not arbitrary firewall rules.

## Commands
- Use `sh ./scripts/check.sh` for the CI-equivalent Go checks; it runs `gofmt -l .`, `go test ./...`, and `go vet ./...`.
- Build the CLI with `go build -o ./bin/cnftctl ./cmd/cnftctl`.
- Focused tests use normal Go package targeting, e.g. `go test ./internal/render` or `go test ./internal/app -run TestName`.
- Render snapshots are updated with `UPDATE_SNAPSHOTS=1 go test ./internal/render`; snapshot fixtures can intentionally preserve exact generated whitespace.
- The reference nftables config includes absolute `/etc/nftables.d/*.nft` paths, so validate it from a staged `/etc` layout or disposable host with `nft -c -f /etc/nftables.conf`, not directly from the repo checkout.

## CI And Release
- Active automation is defined by the files currently under `.github/workflows/`; do not infer workflow state from old documentation.
- The supported delivery unit is the complete Debian 13 amd64 bundle, never a standalone binary. Exact-artifact evidence requirements are in `docs/release-process.md` and `docs/manual-validation.md`.
- Release automation must use least privilege, immutable third-party action pins, and no secret-bearing untrusted PR execution.
- Versioning uses SemVer and commit messages use Conventional Commits by convention; commit-message lint enforcement is not required.
- `.opencode/plans/` is ignored local planning state; do not rely on it being present for future clones.

## Firewall Safety Constraints
- Do not replace targeted cleanup of the managed table with `flush ruleset`; Docker may own other nftables tables.
- Desired config is mutable operator intent; active policy comes only from immutable content-addressed generations. Never blur or bypass that boundary.
- Active policy changes must validate exact final bytes, write a durable generation/transaction, arm and verify rollback before activation through `cnftctl-firewall.service`, and require `cnftctl confirm`.
- Do not document or implement bypasses around rollback except explicit dry-run/test paths.
- SSH should remain open by default; hardening to whitelist-only or whitelist-rate-limit must be explicit and explain lockout risk.
- Any open port is public WAN exposure for both host services and Docker-published services when Docker gating is enabled.
- Docker daemon edits or restarts require explicit consent; Docker integration is opt-in.
- DDNS hostnames are part of the SSH trust boundary. A records are exact IPv4 entries; AAAA records derive IPv6 prefixes, default `/56`, `/64` only for single-LAN trust.
- The only supported production platform is Debian 13 amd64 with systemd and nftables. Do not broaden support claims without release evidence.
- For hardened SSH, preserve current-session coverage checks and the explicit audited override; override must never disable rollback.
- Presets are untrusted input: they may pre-fill config but must not bypass validation, risk explanation, local confirmation, or rollback.

## Sanitization And Scripts
- Never commit real Cloudflare tokens, real domains, private keys, private infrastructure addresses, or personal allowlists; examples must stay sanitized (`example.com`, `203.0.113.0/24`, `198.51.100.0/24`, `2001:db8::/32`).
- Keep shell scripts POSIX `sh` compatible and dependency-light. The server DDNS updater intentionally uses `nft getent awk sort systemd`; the EdgeRouter Cloudflare updater intentionally uses `sh curl ip awk logger` and avoids `jq`/Python.
- `reference/ddns-whitelist/update-nft-ddns-whitelist` is the server-side updater run by systemd; `reference/cloudflare-ddns.sh` is an EdgeOS/EdgeRouter Cloudflare updater, not a server-side script.
