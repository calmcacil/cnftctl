# cnftctl Technical Spec

## Purpose

`cnftctl` manages a specific nftables firewall profile for Linux hosts. It should be able to install the profile on a new machine, inspect an existing installation, and safely manage day-to-day policy changes such as opening/closing public ports and maintaining SSH allowlists.

The current `reference/` package is the baseline behavior. The Go implementation should preserve the security model while making optional features explicit and safer to operate.

## Baseline Firewall Model

The generated firewall should default to:

- Default-deny host input.
- Allow ICMP and ICMPv6 for diagnostics, IPv6 NDP, and Path MTU Discovery.
- Allow loopback.
- Allow established and related traffic.
- Drop invalid TCP, but do not globally drop invalid UDP.
- Apply WAN-scoped uRPF anti-spoofing.
- Keep SSH open by default to avoid accidental lockout.
- Allow public host services only when listed in `open_ports`.
- Never use `flush ruleset`; manage only the app-owned table to avoid destroying Docker-managed nftables state.

The app-owned table should remain `inet hostfw` unless there is a strong reason to make it configurable.

## Core Concepts

### Open Ports

Open ports are public WAN allow rules stored in an nftables set equivalent to `reference/nftables.d/open-ports.nft`:

```nft
set open_ports {
    typeof meta l4proto . th dport
    flags interval
    elements = {
        tcp . 443,
    }
}
```

Each entry is a tuple of protocol and port or port range. Supported protocols for v1 should be `tcp` and `udp`.

Important behavior: an open port allows WAN traffic to both host services and, when Docker integration is enabled, Docker-published services. CLI output should make that exposure clear.

### Static SSH Whitelist

Static allowlist entries are IPv4/IPv6 hosts or prefixes that may reach SSH on port 22 from WAN. This corresponds to `reference/nftables.d/whitelist.nft`.

SSH must remain open by default. Hardening SSH to whitelist-only or whitelist-plus-rate-limit should be an explicit action, not the default install behavior.

### Dynamic DDNS SSH Whitelist

Dynamic allowlist entries are hostnames resolved periodically into nftables runtime sets:

- A records become exact IPv4 elements.
- AAAA records become IPv6 prefixes using a configured prefix length.
- Default IPv6 prefix length is `/56` to match the current DHCPv6-PD model.
- `/64` should be available for a single-LAN trust model.
- Entries should expire automatically if refresh stops.

### Trusted Overlay Interfaces

Trusted interfaces are VPN or overlay network interfaces such as Tailscale, WireGuard, or NetBird. In the first implementation stage, enabling one of these interfaces grants full trust to traffic on that interface.

This should be opt-in. Defaults should not assume `tailscale0` exists or should be trusted.

### Docker WAN Gate

Docker integration gates WAN-to-Docker traffic by the same `open_ports` set while preserving Docker's own forwarding behavior. This should be opt-in because not every host runs Docker and because the rule has broader exposure implications. `init` may auto-detect Docker and recommend enabling integration, but it must not enable it silently.

When enabled, the rules should support:

- IPv4 DNATed Docker-published ports using the original destination port.
- IPv6 DNATed Docker-published ports.
- IPv6 routed container services by destination port.

## Installation Targets

Default managed paths should match the current reference deployment:

- `/etc/nftables.conf`
- `/etc/nftables.d/open-ports.nft`
- `/etc/nftables.d/whitelist.nft`
- `/etc/nftables.d/ddns-hosts.conf`
- `/usr/local/sbin/update-nft-ddns-whitelist`
- `/etc/systemd/system/nft-ddns-whitelist.service`
- `/etc/systemd/system/nft-ddns-whitelist.timer`

The CLI should support dry-run and alternate output roots for testing and previewing generated files.

## Configuration

The tool should keep a small declarative config file rather than only editing rendered `.nft` fragments. Proposed path:

```text
/etc/cnftctl/config.yaml
```

Proposed shape:

```yaml
wan_interface: eth0

open_ports: []

ssh:
  mode: open
  rate_limit: null
  static_whitelist:
    ipv4: []
    ipv6: []
  ddns_whitelist:
    enabled: false
    hosts: []
    ttl: 1h
    refresh_interval: 5m
    ipv6_prefix_len: 56

trusted_interfaces:
  enabled: false
  interfaces: []
  trust_forwarding: false

docker:
  enabled: false
  allow_published_ports_by_default: false
  interfaces:
    - docker0
    - br-*
```

The renderer should generate nftables/systemd files from this config. If managing an existing reference install, `cnftctl` should be able to import current files into config where practical.

## CLI Commands

Command names should prioritize clear operational tasks.

The binary name is `cnftctl`, short for calmcacil+nftctl. Avoid installing a binary named `nftctl`: that name has direct public collisions. `deranok/nftctl` ships a root shell executable named `nftctl` for starting/stopping/listing nftables rules, and `enDzioQ/nftctl` ships a similar firewall manager as `nftctl.sh` with port whitelist commands.

### Inspection

```sh
cnftctl status
cnftctl config show
cnftctl ports list
cnftctl whitelist list
```

`status` should report whether the managed table exists, whether nftables validation passes, whether optional services/timers are enabled, and which optional modules are active.

### New Machine Setup

```sh
cnftctl init
cnftctl init --dry-run
cnftctl init --wan-interface eth0
cnftctl init --enable-docker
cnftctl init --trust-interface tailscale0
cnftctl init --enable-ddns-whitelist
cnftctl init --preset eyJ2ZXJzaW9uIjoxLCJvcGVuX3BvcnRzIjpbXX0
cnftctl init --preset-file preset.json
```

`init` should generate config and managed files, validate them with `nft -c -f`, and require confirmation before applying unless `--yes` is passed. A fresh install should open no public WAN service ports beyond SSH.

Remote safety is mandatory. Any command that changes active firewall policy must start a rollback/dead-man timer, defaulting to 120 seconds. The CLI must clearly state that `cnftctl confirm` is required before the timeout or the previous known-good files/rules will be restored. Interactive sessions may also show a visible countdown; if the SSH session terminates before confirmation, rollback should still occur.

### Apply And Validate

```sh
cnftctl validate
cnftctl plan
cnftctl apply
cnftctl apply --dry-run
cnftctl apply --rollback-timeout 120s
```

`plan` should show file and rule changes without applying them. `apply` should write files atomically, validate before loading, load with nftables, start the 120 second rollback timer, and roll back automatically unless confirmed with `cnftctl confirm` before the timeout expires.

### Presets

```sh
cnftctl preset decode eyJ2ZXJzaW9uIjoxLCJvcGVuX3BvcnRzIjpbXX0
cnftctl preset validate preset.json
cnftctl preset explain preset.json
```

Presets should pre-fill init/config choices without bypassing local validation, confirmation, or dead-man rollback. A preset imported through `init --preset` or `init --preset-file` must be decoded, summarized, and confirmed before any files are written or policy is applied.

The preset format should be versioned JSON. The copy/paste string should be base64url-encoded JSON so it is URL-safe and easy for the CLI to decode. Compression may be added later if real presets become too large.

Preset payloads may include:

- WAN interface preference.
- SSH hardening mode and rate-limit settings.
- Open port entries.
- Static whitelist entries.
- DDNS whitelist enablement, hostnames, TTL, refresh interval, and IPv6 prefix length.
- Trusted interface settings.
- Docker integration settings.

The CLI should treat presets as untrusted input. It must reject unknown schema versions, validate all values, and show warnings for risky settings such as broad static allowlist prefixes, Docker integration, Docker daemon changes, SSH hardening, or public open ports.

### GitHub Pages Preset Builder

The project may include a GitHub Pages preset builder for creating reusable install presets before logging into a server.

Requirements:

- Static browser-only application.
- No backend service.
- No form submissions.
- No analytics by default.
- No network calls after the page loads, except browser requests needed to load the static site assets.
- Clearly show the decoded JSON before export.
- Support import, edit, validate, explain, and export flows.
- Generate both a readable JSON file and a base64url preset string.
- Use the same schema as the CLI where practical.

Because the builder is browser-only, secrets are not sent to a project server. Even so, the UI should warn users that presets are easy to copy, share, log, or store in shell history. Secrets are technically possible but should be discouraged unless there is a concrete use case and the user understands the exposure risk. The first implementation should not require Cloudflare tokens or private keys in presets.

### Open Port Management

```sh
cnftctl open tcp 443
cnftctl open udp 41641 --comment "Tailscale direct connectivity"
cnftctl close tcp 443
cnftctl ports list
```

`open` and `close` should update the managed `open-ports.nft` representation immediately, but they must not reload active nftables policy. The user must run `cnftctl apply` to validate, load, and start the mandatory dead-man rollback flow.

Validation rules:

- Protocol must be `tcp` or `udp`.
- Port must be `1..65535`; ranges may be supported as `start-end` after single ports work.
- Duplicate opens should be idempotent.
- Closing a missing port should be a clear no-op unless `--strict` is passed.
- Output should warn when Docker integration is enabled because open ports also affect Docker-published services.

### Static SSH Whitelist Management

```sh
cnftctl whitelist add 203.0.113.10
cnftctl whitelist add 2001:db8::10
cnftctl whitelist add 198.51.100.0/24 --comment "office"
cnftctl whitelist remove 203.0.113.10
cnftctl whitelist list
```

Validation rules:

- Accept IPv4, IPv6, and CIDR prefixes.
- Reject hostnames here; hostnames belong in DDNS whitelist commands.
- Warn when adding broad prefixes.

### DDNS Whitelist Management

```sh
cnftctl ddns enable
cnftctl ddns disable
cnftctl ddns add home.example.com
cnftctl ddns remove home.example.com
cnftctl ddns refresh
cnftctl ddns status
cnftctl ddns set-ipv6-prefix-len 64
```

`ddns refresh` should run the updater once or perform the equivalent logic directly. `ddns status` should show configured hosts, resolved current entries, timer state, and nftables set contents when available. A Go-based refresh daemon is preferred long-term because it removes the POSIX shell updater and lets systemd start the app directly, but keeping the current script is acceptable for an initial implementation if it reduces scope.

### SSH Hardening

```sh
cnftctl ssh-harden whitelist-only
cnftctl ssh-harden whitelist-rate-limit
cnftctl ssh-harden open
```

SSH should be open by default to avoid accidental lockout. `ssh-harden` should make the risk explicit, require existing whitelist coverage or an explicit override, and then require `cnftctl apply` for the active firewall change. Supported modes for v1 should be open, whitelist-only, and whitelist-plus-rate-limit.

### Optional Feature Toggles

```sh
cnftctl feature enable docker
cnftctl feature disable docker
cnftctl feature enable trusted-interface --interface tailscale0
cnftctl feature enable trusted-interface --interface wg0
cnftctl feature enable trusted-interface --interface wt0
cnftctl feature disable trusted-interface --interface tailscale0
```

Toggles should update config and require `apply` unless a command has an explicit `--apply` flag.

When enabling Docker integration, the CLI should explain that Docker-published ports remain blocked from WAN until the matching protocol/port is added with `cnftctl open`. Longer-term, `cnftctl` may support an explicit option to allow Docker-published ports by default, but the safe default is to require `open_ports` entries.

Docker Engine defaults to creating firewall rules with iptables, but supports nftables for bridge networks via the daemon option `"firewall-backend": "nftables"`. When enabling Docker integration, `cnftctl` should check or warn about Docker's firewall backend. If Docker has been running with iptables, restarting it with the nftables backend deletes most Docker iptables chains/rules and creates Docker nftables rules instead. This migration requires user consent and should be presented as a disruptive operation.

Because apply/setup commands already require root, the CLI may offer to update Docker's daemon config with user consent. If `/etc/docker/daemon.json` exists and does not enable the nftables backend, `cnftctl` should show the proposed JSON change, create a timestamped backup, validate the resulting JSON, and ask before restarting Docker. Docker restart is required for the backend change to take effect and can disrupt running containers, so this must never happen implicitly. After Docker is configured for nftables, normal port changes should not require restarting Docker because Docker manages its own nftables rules.

## Suggested Uplifts From Reference

- Make Docker WAN gating opt-in instead of assuming Docker host behavior.
- Make trusted VPN/overlay interfaces opt-in instead of defaulting to `tailscale0`.
- Make DDNS whitelist optional and disabled until configured.
- Keep SSH open by default, with an explicit `ssh-harden` workflow for whitelist-only or whitelist-plus-rate-limit modes.
- Add dry-run, plan, and validation output before applying firewall changes.
- Require a 120 second rollback/dead-man timer for every command that changes active firewall policy.
- Generate files from a typed config rather than requiring hand edits to nft fragments.
- Preserve comments/labels for managed open ports and whitelist entries in config, even if rendered nftables comments are limited.
- Support import/adopt of an existing reference install.
- Detect the default WAN interface from `ip route show default`, but require confirmation before writing it.
- Check required binaries before enabling features: `nft`, `systemctl`, `getent`, and any future resolver dependency.
- Offer consent-gated Docker `/etc/docker/daemon.json` updates to enable the nftables backend when Docker integration is enabled.
- Validate that generated files do not contain placeholder secrets or real Cloudflare credentials before packaging examples.

## Non-Goals For V1

- General-purpose firewall management unrelated to this profile.
- Managing arbitrary nftables tables, chains, or user-defined rules.
- Managing Docker itself or published container port definitions.
- Managing Cloudflare DNS records from the server-side CLI.
- Replacing EdgeRouter-specific DDNS behavior beyond keeping the reference script documented.
- Supporting non-systemd service managers unless a real target host needs it.

## Implementation Notes

- Use Go for the CLI and file rendering.
- Prefer small, explicit renderers over templating that obscures nftables semantics.
- Keep generated nftables compatible with Docker-managed tables by deleting/recreating only `inet hostfw`.
- Treat `/etc` writes and nftables loads as privileged operations with clear errors when not root.
- Keep shell scripts POSIX `sh` compatible if retained for DDNS refresh.
- Tests should cover config parsing, validation, rendering, import/adoption, and command behavior without requiring root or live nftables by default.

## Open Questions

- Should managed nftables fragments live directly under `/etc/nftables.d/`, or under `/etc/cnftctl/nftables.d/` with `/etc/nftables.conf` including them?
- Should v1 keep the POSIX DDNS updater script, or implement DDNS refresh directly in Go from the start?
- Should long-term Docker support include an opt-in mode where Docker-published ports are allowed by default without matching `open_ports` entries?
