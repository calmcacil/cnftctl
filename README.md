# cnftctl

`cnftctl` is the Go-based control tool for a specific nftables firewall profile used on Linux hosts. It is not a general-purpose firewall manager. The first release is intended to make the existing sanitized reference firewall repeatable, reviewable, and safer to operate with explicit validation and dead-man rollback expectations.

The baseline behavior comes from `reference/`: default-deny host input, public WAN ports managed through one `open_ports` set, optional Docker WAN gating, optional DDNS SSH allowlists, and no use of `flush ruleset`.

## Status

This repository now has a Go module for `github.com/calmcacil/cnftctl` and first-release scaffolding. Some command-package implementation may still be in progress; use `go test ./...` as the source of truth for the current code state.

Current release surface:

- `reference/` remains the known-good sanitized deployment package and behavior reference.
- `SPEC.md` records the intended CLI behavior and security model.
- `docs/` contains operator-facing first-release notes and a manual validation checklist.
- `examples/` contains sanitized config and preset examples.
- `.github/workflows/` contains active CI; disabled future release scaffolding is kept outside active workflows under `.github/workflows-disabled/`.

## Safety Warning

Firewall changes can lock you out of a remote host. Do not apply this firewall over SSH unless you have an out-of-band recovery path such as console access, rescue mode, IPMI, a cloud serial console, or a tested rollback path.

The planned active-policy flow for `cnftctl apply` is:

- Validate generated nftables before loading.
- Write managed files atomically.
- Load the managed firewall policy.
- Start a rollback/dead-man timer, defaulting to 120 seconds.
- Require `cnftctl confirm` before the timer expires.
- Restore the previous known-good files/rules if confirmation does not happen.

Until that flow is implemented and verified end-to-end, treat `reference/` deployment as manual firewall work and use your own dead-man rollback procedure.

## Requirements

Runtime requirements for a managed host:

- Linux with nftables.
- Root privileges for install, apply, service management, and nftables loads.
- `nft` for validation and loading.
- `systemd`/`systemctl` for the DDNS whitelist timer if that feature is enabled.
- Docker only when Docker WAN gating is enabled.

Development requirements:

- Go 1.22 or newer matching `go.mod`.

## Build And Test

Run local checks from the repository root:

```sh
go test ./...
go vet ./...
test -z "$(gofmt -l .)"
```

Build the CLI with:

```sh
go build -o ./bin/cnftctl ./cmd/cnftctl
```

Install a locally built binary with:

```sh
sudo install -m 0755 ./bin/cnftctl /usr/local/bin/cnftctl
```

CI runs the same checks plus the CLI build on pull requests and pushes to `main`.

## First-Run Flow

The intended first-run CLI flow is:

```sh
cnftctl init --dry-run
sudo cnftctl init --wan-interface eth0
cnftctl plan
sudo cnftctl apply
sudo cnftctl confirm
cnftctl status
```

Important first-run defaults and expectations:

- A fresh install should open no public WAN service ports beyond the selected SSH mode.
- SSH should remain open by default to reduce accidental lockout risk.
- SSH hardening to whitelist-only or whitelist-plus-rate-limit must be explicit.
- Optional Docker gating, DDNS whitelist, and trusted overlay interfaces must be enabled intentionally.
- `open` and `close` style commands should update config/rendered files, but active nftables policy should not change until `apply` succeeds and is confirmed.

The current manual reference deployment flow is documented in `reference/README.md`.

## Firewall Model

The managed firewall profile is expected to preserve these behaviors:

- Manage only the app-owned `inet hostfw` table.
- Avoid `flush ruleset` so Docker-managed nftables state is not destroyed.
- Default-deny host input.
- Allow ICMP/ICMPv6 for diagnostics, IPv6 NDP, and Path MTU Discovery.
- Allow loopback and established/related traffic.
- Drop invalid TCP without globally dropping invalid UDP.
- Apply WAN-scoped uRPF anti-spoofing.
- Allow public WAN service ports only when listed in `open_ports`.
- Use static and optional DDNS allowlists for SSH hardening modes.

## Docker Caveats

Docker integration is opt-in because it changes exposure semantics and depends on Docker's firewall backend.

When Docker WAN gating is enabled:

- Every entry in `open_ports` is public from WAN for matching host services and Docker-published services.
- Docker can publish ports that remain blocked from WAN until the matching protocol/port is listed in `open_ports`.
- Docker Engine may need the nftables firewall backend for the intended behavior.
- Changing Docker's firewall backend can require editing `/etc/docker/daemon.json` and restarting Docker, which can disrupt running containers.
- `cnftctl` must not restart Docker or rewrite Docker daemon configuration without explicit consent.

## DDNS IPv6 Prefix Behavior

The DDNS SSH whitelist treats DNS hostnames as part of the SSH trust boundary.

Expected behavior:

- A records become exact IPv4 whitelist entries.
- AAAA records are converted into IPv6 prefixes.
- The default IPv6 prefix length is `/56`, matching DHCPv6-PD setups where a router receives a delegated `/56` and assigns `/64` LANs.
- Use `/64` only when you want to trust a single LAN prefix rather than the broader delegated prefix.
- Dynamic entries should expire automatically if refresh stops.

The reference updater implements this with `IPV6_PREFIXLEN="56"` in `reference/ddns-whitelist/update-nft-ddns-whitelist`.

## Presets

Presets are planned as versioned JSON payloads that can be passed as readable JSON files or base64url-encoded strings. They are intended to pre-fill configuration, not to bypass local validation or confirmation.

Security rules for presets:

- Treat presets as untrusted input.
- Reject unknown schema versions.
- Validate every port, protocol, CIDR, hostname, interface, duration, and feature flag.
- Explain risky choices before writing files, including SSH hardening, broad allowlist prefixes, Docker integration, Docker daemon changes, public open ports, and DDNS trust.
- Do not put Cloudflare tokens, private keys, passwords, or personal IP allowlists in presets.

See `examples/config.yaml` and `examples/preset.v1.json` for sanitized examples.

## Documentation

- `SPEC.md` - technical design and target command behavior.
- `docs/release-notes.md` - first-release notes and release checklist.
- `docs/manual-validation.md` - manual validation checklist for packaging and host testing.
- `docs/release-process.md` - SemVer, Conventional Commits, and disabled release workflow policy.
- `reference/README.md` - current sanitized manual deployment package.
- `reference/nftables.conf` - baseline nftables ruleset.
- `reference/nftables.d/open-ports.nft` - public WAN open-port set.
- `reference/nftables.d/whitelist.nft` - static SSH whitelist examples.

## Security Notes

- Keep examples sanitized. Do not commit real tokens, real domains, private addresses, or personal allowlists.
- Any public open port is a WAN exposure decision.
- Any DDNS hostname is an SSH trust decision.
- Any trusted interface is full-trust for the first-release model.
- Review generated nftables before loading it on a remote host.
