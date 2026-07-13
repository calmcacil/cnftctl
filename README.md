# cnftctl

> Install only a published Debian package whose checksum and GitHub attestation you have verified. See `docs/production-readiness.md` and the versioned validation record for release evidence.

`cnftctl` manages one application-owned nftables profile, `inet hostfw`, on Debian 13 amd64 hosts. It is not a general firewall manager. It keeps operator-edited desired configuration separate from immutable active generations and never uses `flush ruleset`, so unrelated nftables owners such as Docker can coexist.

## Support

The supported production target is exactly:

- Debian 13 (`trixie`), `amd64`.
- systemd and nftables from Debian 13.
- Root for installation and active-policy operations.
- Docker only when its strict WAN gate is explicitly enabled.

Other distributions, releases, architectures, init systems, and nftables versions are untested and reported as unsupported. See `docs/support-matrix.md`.

A release is supported only after its exact Debian package completes the evidence gate in `docs/manual-validation.md` and `docs/release-process.md`. Source checks or results from a different package are not substitutes.

## Safety

Firewall changes can terminate remote access. Before the first apply, arrange console, rescue, IPMI, or cloud serial-console access.

Every non-dry-run `apply`:

1. Validates the exact final generation with `nft -c`.
2. Writes a content-addressed immutable generation under `/var/lib/cnftctl/generations/`.
3. Arms and verifies a pre-installed 120-second systemd rollback timer.
4. Atomically selects the generation and restarts `cnftctl-firewall.service`.
5. Requires `cnftctl confirm TRANSACTION_ID` before the deadline.

An expired first-install transaction deletes only `inet hostfw`. A later expired transaction restores the prior generation. The timer is independent of the invoking shell, and `cnftctl-reconcile.service` rolls back unconfirmed durable transactions after reboot. There is no supported rollback bypass and the timeout is fixed at 120 seconds.

## Install A Release Package

Download `cnftctl_VERSION_amd64.deb` and the accompanying `release-checksums.txt` from the same GitHub release. Verify the checksum and GitHub attestation before installation:

```sh
sha256sum --check release-checksums.txt
gh attestation verify cnftctl_VERSION_amd64.deb --repo calmcacil/cnftctl
sudo apt install ./cnftctl_VERSION_amd64.deb
```

The package enforces Debian 13 amd64, installs the binary, recovery helper, integrity inventory, documentation, and systemd units, and enables only boot reconciliation. It does not activate firewall policy, enable DDNS, or restart Docker. Package upgrades and removals refuse unresolved transaction state; removal also refuses while `inet hostfw` is active. Both `apt remove` and `apt purge` preserve `/etc/cnftctl` and `/var/lib/cnftctl`.

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

Docker is not trusted merely because it is installed. Enabling the feature trusts host input arriving from `docker0` and `br-*`, leaves Docker's own container and bridge forwarding behavior intact, and adds only the narrow WAN-to-Docker gate. cnftctl never flushes or takes ownership of Docker's nftables tables.

`cnftctl docker backend plan` previews setting Docker's `firewall-backend` to `nftables`. `cnftctl docker backend write --yes` preserves other daemon JSON keys and writes a timestamped backup. It never restarts Docker. A backend migration and Docker restart are disruptive and remain an explicit operator action.

## Tailscale And Trusted Interfaces

Tailscale is supported through the generic trusted-interface feature and is never enabled or discovered automatically:

```sh
sudo cnftctl feature enable trusted-interface --interface tailscale0
sudo cnftctl apply
sudo cnftctl confirm TRANSACTION_ID
```

Traffic arriving on `tailscale0` then receives full host-input trust, including SSH. Do not add the entire `100.64.0.0/10` CGNAT range to a WAN whitelist; Tailscale authentication and ACLs remain Tailscale's responsibility. Forwarded VPN trust is a separate explicit `trusted_interfaces.trust_forwarding` setting. UDP port 41641 may be added to `open_ports` for direct connectivity; DERP fallback works without it. cnftctl does not install Tailscale, configure tailnet ACLs, or advertise routes.

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
sh ./scripts/build-deb.sh 0.1.0 ./cnftctl_0.1.0_amd64.deb
```

The bundle builder and reference files remain internal staging and sanitized behavior baselines, not supported installation paths. Contribution terms are in `CONTRIBUTING.md`, vulnerability reporting is in `SECURITY.md`, and third-party attribution is in `THIRD_PARTY_NOTICES.md`.

## Documentation

- `SPEC.md`: implemented architecture and invariants.
- `docs/operator-guide.md`: install, operation, logging, upgrades, uninstall, and recovery.
- `docs/manual-validation.md`: executable validation checklist for an exact release artifact.
- `docs/support-matrix.md`: supported and unsupported environments.
- `docs/incident-response.md`: lockout, rollback, boot, DDNS, and Docker runbooks.
- `docs/release-process.md`: release evidence and publication procedure.
- `docs/release-notes.md`: release evidence template and limitations.
