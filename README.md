# cnftctl

`cnftctl` is a planned Go CLI for installing and managing my personal nftables firewall setup on Linux hosts.

This is not intended to be a universal firewall manager. The goal is to turn a known-good personal firewall configuration into a safer, repeatable tool for my own machines, while sharing the work publicly in case the approach is useful to someone else.

## Current Status

The repository currently contains:

- `reference/` - the sanitized nftables, DDNS whitelist, systemd, and EdgeRouter scripts that describe the current manual setup.
- `SPEC.md` - the technical direction for the future `cnftctl` CLI.
- `AGENTS.md` - repo-specific guidance for future OpenCode sessions.

There is not a Go module, build, or released CLI yet.

## What This Project Is Trying To Do

The intended CLI should help with:

- Bootstrapping this nftables firewall profile on a new Linux machine.
- Managing public WAN open ports with commands like `cnftctl open tcp 443` and `cnftctl close tcp 443`.
- Keeping SSH safe by default, with explicit hardening options for whitelist-only or whitelist-plus-rate-limit modes.
- Managing static SSH allowlists.
- Optionally managing DDNS-based SSH allowlists.
- Optionally trusting overlay/VPN interfaces such as Tailscale, WireGuard, or NetBird.
- Optionally gating Docker-published services through the same public open-port policy.
- Applying firewall changes with a mandatory dead-man rollback timer to reduce the risk of remote lockout.

## Baseline Firewall Model

The reference firewall is a default-deny host firewall that:

- Allows ICMP/ICMPv6 for diagnostics, NDP, and Path MTU Discovery.
- Allows loopback and established/related traffic.
- Keeps SSH open by default to avoid accidental lockout.
- Allows public WAN services only when listed in `open_ports`.
- Can gate Docker-published services by the same `open_ports` set.
- Avoids `flush ruleset` so Docker-managed nftables state is not destroyed.

See `reference/README.md` for the current manual deployment behavior.

## Safety Goals

Firewall tooling can easily lock out a remote administrator. `cnftctl` should make safety a core behavior, not an optional extra.

Planned safety behavior:

- Validate generated nftables rules before loading them.
- Show a plan/dry-run before applying changes.
- Start a rollback timer for active firewall changes.
- Require `cnftctl confirm` before the timer expires.
- Restore the previous known-good rules/files if confirmation does not happen.
- Avoid changing Docker daemon configuration without explicit consent.

## For Other People

You are welcome to read, copy, or adapt the ideas here, but expect assumptions that match my environment and threat model. Review the reference rules carefully before using them on your own systems.

In particular:

- Open ports become public from WAN.
- SSH hardening decisions can lock you out if your allowlist is wrong.
- Docker firewall behavior depends on Docker's firewall backend and version.
- DDNS whitelist entries become part of your SSH trust boundary.

## Documentation

- `SPEC.md` - planned CLI behavior and design decisions.
- `reference/README.md` - current sanitized firewall deployment package.
- `reference/nftables.conf` - baseline nftables ruleset.
- `reference/nftables.d/open-ports.nft` - public WAN open-port set.
- `reference/nftables.d/whitelist.nft` - static SSH whitelist examples.
