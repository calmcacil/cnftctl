# cnftctl

> Production status: **NOT READY** pending exact Debian 13 amd64 artifact validation. See `docs/production-readiness.md` for remaining work and `docs/validation-record.md` for the required evidence record.

`cnftctl` manages one application-owned nftables profile, `inet hostfw`, on Debian 13 amd64 hosts. It is not a general firewall manager. It keeps operator-edited desired configuration separate from immutable active generations and never uses `flush ruleset`, so unrelated nftables owners such as Docker can coexist.

## Support

The supported production target is exactly:

- Debian 13 (`trixie`), `amd64`.
- systemd and nftables from Debian 13.
- Root for installation and active-policy operations.
- Docker only when its strict WAN gate is explicitly enabled.

Other distributions, releases, architectures, init systems, and nftables versions are untested and reported as unsupported. See `docs/support-matrix.md`.

**Production status: NOT READY.** This repository documents the approved architecture contract, but no release is production-ready until the canonical Debian 13 amd64 artifact has the complete exact-artifact evidence required by `docs/manual-validation.md` and `docs/release-process.md`. Do not interpret implemented code or this documentation as evidence that those checks passed.

## Safety

Firewall changes can terminate remote access. Before the first apply, arrange console, rescue, IPMI, or cloud serial-console access.

Every non-dry-run `apply`:

1. Validates the exact final generation with `nft -c`.
2. Writes a content-addressed immutable generation under `/var/lib/cnftctl/generations/`.
3. Arms and verifies a pre-installed 120-second systemd rollback timer.
4. Atomically selects the generation and restarts `cnftctl-firewall.service`.
5. Requires `cnftctl confirm TRANSACTION_ID` before the deadline.

An expired first-install transaction deletes only `inet hostfw`. A later expired transaction restores the prior generation. The timer is independent of the invoking shell, and `cnftctl-reconcile.service` rolls back unconfirmed durable transactions after reboot. There is no supported rollback bypass and the timeout is fixed at 120 seconds.

## Install A Release Bundle

Use the complete release bundle, not a binary copied by itself. The bundle contains the binary, systemd units, manifest, checksums, installer, and uninstaller.

```sh
tar -xf cnftctl-VERSION-debian13-amd64.tar.gz
cd cnftctl-VERSION-debian13-amd64
./scripts/verify-bundle .
sudo ./install.sh
```

The installer contract verifies bundle checksums and the Debian 13 amd64 platform, installs `/usr/bin/cnftctl` and units under `/usr/lib/systemd/system`, and does not activate firewall policy. Guarded upgrade and uninstall are part of that contract. Production support remains withheld until these behaviors pass exact-artifact validation.

## First Policy

```sh
sudo cnftctl init --wan-interface eth0 --yes
sudo cnftctl validate
sudo cnftctl plan
sudo cnftctl apply
# Verify this SSH session and a second administrative path.
sudo cnftctl confirm TRANSACTION_ID
sudo cnftctl status
```

`init`, `open`, `close`, whitelist/DDNS edits, SSH mode changes, and feature toggles update only mutable desired operator intent in `/etc/cnftctl/config.yaml`. Desired config is not active policy and editing it does not change live nftables rules. `apply` creates and activates an immutable generation; `confirm` makes that generation survive its rollback deadline.

SSH is open from WAN by default to reduce first-install lockout risk. Hardened modes require allowlist coverage. When an existing SSH session is not covered, apply fails unless the operator explicitly supplies both `--acknowledge-ssh-lockout-risk` and a non-empty `--reason`, for interactive and noninteractive use alike. The acknowledgement and reason are recorded in transaction state.

## Policy Model

- Default-deny host input with loopback, established/related traffic, ICMP/ICMPv6, IPv6 NDP, and Path MTU Discovery allowed.
- Invalid TCP is dropped without a global invalid-UDP drop.
- WAN-scoped reverse-path anti-spoofing is applied.
- `open_ports` entries expose matching host services publicly from WAN.
- Trusted interfaces are opt-in and fully trusted for configured input behavior.
- Static SSH entries accept IP addresses and CIDRs; hostnames belong to DDNS.
- DDNS A records become exact IPv4 entries. AAAA records become `/56` prefixes by default; `/64` is the only alternative and trusts one LAN prefix.
- DDNS refresh uses runtime nftables sets with timeouts and records freshness metadata. The timer unit is installed but remains disabled until selected active-generation intent enables it; its state is reconciled after activation, rollback, and boot.

## Docker

Docker integration is disabled by default and is a strict WAN gate. When enabled, a Docker-published service is reachable from WAN only when its protocol and public port also appear in `open_ports`; that same entry exposes a matching host service. IPv4 DNAT uses the original destination port, while supported IPv6 DNAT/routed traffic is gated by destination port.

`cnftctl docker backend plan` previews setting Docker's `firewall-backend` to `nftables`. `cnftctl docker backend write --yes` preserves other daemon JSON keys and writes a timestamped backup. It never restarts Docker. A backend migration and Docker restart are disruptive and remain an explicit operator action.

## Inspection And Automation

`status`, `doctor`, `validate`, `plan`, `transactions list`, and `ddns status` support `--output text|json`; `--detail` includes additional potentially sensitive values. `transactions list` reports pending transactions, not transaction history. JSON uses schema `cnftctl.report.v1` with stable check IDs, states, summaries, optional codes/details, and command-specific data.

Exit codes are:

| Code | Meaning |
| --- | --- |
| `0` | Command succeeded; an inspection is healthy or not applicable. |
| `1` | Inspection completed and emitted usable output, but found absent, pending, degraded, failed, unknown, or unsupported state. |
| `2` | Usage, validation, permission, I/O, or operational failure. |

Use `journalctl -u cnftctl-firewall.service`, `journalctl -u cnftctl-reconcile.service`, `journalctl -u 'cnftctl-rollback@*.service'`, and `journalctl -u cnftctl-ddns-refresh.service` for service logs. See `docs/operator-guide.md` and `docs/incident-response.md`.

## Development

```sh
sh ./scripts/check.sh
go build -o ./bin/cnftctl ./cmd/cnftctl
```

The reference files remain a sanitized behavior baseline, not the supported bundle installation path. Contribution terms are in `CONTRIBUTING.md`, vulnerability reporting is in `SECURITY.md`, and third-party attribution is in `THIRD_PARTY_NOTICES.md`.

## Documentation

- `SPEC.md`: implemented architecture and invariants.
- `docs/operator-guide.md`: install, operation, logging, upgrades, uninstall, and recovery.
- `docs/manual-validation.md`: executable validation checklist for an exact release artifact.
- `docs/support-matrix.md`: supported and unsupported environments.
- `docs/incident-response.md`: lockout, rollback, boot, DDNS, and Docker runbooks.
- `docs/release-process.md`: release evidence and publication procedure.
- `docs/release-notes.md`: release evidence template and limitations.
