# cnftctl Implemented Architecture

## Scope And Invariants

`cnftctl` exclusively manages the app-owned `inet hostfw` nftables table on Debian 13 amd64. It does not manage arbitrary rules, Docker containers, DNS provider records, routing policy, or other nftables tables. It never issues `flush ruleset`.

The policy provides default-deny host input, loopback and established/related acceptance, required ICMP/ICMPv6 behavior, WAN reverse-path filtering, explicit public ports, three SSH exposure modes, optional trusted interfaces, optional DDNS SSH sources, and optional strict Docker WAN gating.

## State Model

The state layers have different authority:

| State | Path | Role |
| --- | --- | --- |
| Desired config | `/etc/cnftctl/config.yaml` | Mutable operator intent; strict versioned YAML. |
| Generations | `/var/lib/cnftctl/generations/SHA256/` | Content-addressed rendered files plus `manifest.json`; written durably and made read-only. |
| Active selector | `/var/lib/cnftctl/active` | Atomic symlink selecting one generation. |
| Transactions | `/var/lib/cnftctl/transactions/ID/state.json` | Durable apply phase, deadline, old/new generation, confirmation, rollback, and SSH override audit. |
| Ownership | `/var/lib/cnftctl/ownership.json` | Binds `inet hostfw` and its marker to the selected generation. |
| Runtime lock | `/run/cnftctl/apply.lock` | Serializes apply, rollback, reconciliation, and DDNS set replacement. |

Desired edits do not mutate a generation or live rules. A generation hash is computed from a semantic manifest; embedded generation paths are normalized before hashing. Existing generations are hash-verified before reuse.

## Activation Transaction

The only normal active-policy transition is `cnftctl apply`:

1. Load and validate desired config.
2. Render final-shaped generation files.
3. Validate the exact candidate entrypoint with `nft -c`.
4. Acquire the runtime lock and reject concurrent or pending applies.
5. Verify installed delivery assets and existing `inet hostfw` ownership.
6. Durably write generation files, manifest, and prepared transaction state.
7. Start and verify `cnftctl-rollback@ID.timer` before changing the selector.
8. Atomically update `/var/lib/cnftctl/active`.
9. Restart the dedicated `cnftctl-firewall.service`, which loads `/var/lib/cnftctl/active/firewall.nft`.
10. Record ownership and reconcile the DDNS refresh timer with generation intent.
11. Require confirmation before the fixed 120-second deadline.

`confirm` durably marks the transaction confirmed and stops its rollback timer. On timeout, a fresh install deletes only `table inet hostfw` and removes the active selector; an update selects and loads the previous generation. Rollback also restores the prior generation's DDNS timer intent. Activation failures invoke the same recovery path.

The rollback is systemd-owned, not attached to the initiating process. `cnftctl-reconcile.service` runs during boot and rolls back every unconfirmed durable transaction. The firewall service is a dedicated oneshot unit ordered before `network.target`; it is not `/etc/nftables.conf` or the distribution `nftables.service`.

## SSH Safety Override

For hardened SSH modes, apply checks the client address from `SSH_CONNECTION` or `SSH_CLIENT` against static allowlists, current DDNS resolution, and a configured trusted-interface server address. An uncovered session is rejected by default.

`--acknowledge-ssh-lockout-risk` permits an explicit override only when accompanied by a non-empty `--reason`, regardless of whether invocation is interactive. Acknowledgement, reason, CLI source, and connection context are stored in transaction state with bounded lengths. This override does not disable rollback.

## DDNS Policy

DDNS is disabled by default. Enabling it requires at least one syntactically valid hostname. A records produce exact IPv4 set elements. AAAA addresses are masked to `/56` by default or `/64` when explicitly selected. Runtime elements carry the configured TTL.

Refresh resolves all configured names before replacing both nftables sets under the firewall lock. Attempt metadata is written even when refresh fails, allowing `status` and `doctor` to report stale or failed runtime state. The installed `cnftctl-ddns-refresh.timer` is enabled for boot. Reconciliation starts or stops its runtime activity according to the selected generation's DDNS intent after activation, rollback, and boot; desired config alone has no authority over the timer.

## Docker Policy

Docker support is opt-in and strict: WAN-to-Docker forwarding is denied unless the protocol/port tuple exists in `open_ports`. IPv4 DNAT compares the original public destination port. IPv6 DNAT and routed container traffic use the applicable destination-port gate. The forwarding chain otherwise preserves Docker's own forwarding behavior and unrelated Docker tables are untouched.

Daemon configuration inspection and writing are separate from firewall apply. Backend writes preserve valid JSON, create a timestamped backup, require `--yes`, and never restart Docker.

## Configuration And Commands

Config schema version is `1`. Unknown fields and versions are rejected. Ports support TCP/UDP single ports and ranges. Static whitelist entries are structured values with optional comments. Docker has only `enabled` and `interfaces`; there is no allow-published-by-default mode.

Policy-mutating commands change mutable desired operator intent only: `init`, `open`, `close`, `whitelist`, `ddns` config commands, `ssh-harden`, `feature`, and `adopt reference`. Inspection and lifecycle commands include `validate`, `plan`, `apply`, `confirm`, `rollback`, `reconcile`, `transactions list`, `status`, and `doctor`. `transactions list` reports pending transactions rather than durable history. `doctor` currently runs the same comprehensive inspection as `status`.

Presets are versioned JSON wrappers around the same strict config. They may initialize desired state but cannot activate it or bypass review, validation, SSH checks, or rollback.

## Reports And Exit Codes

Inspection reports support text and JSON. JSON schema `cnftctl.report.v1` contains `command`, conservative overall `state`, `checks`, and optional `data`. States are `ok`, `absent`, `pending`, `degraded`, `failed`, `unknown`, `unsupported`, and `not_applicable`. Sensitive addresses and hostnames are withheld unless `--detail` is supplied where implemented.

Exit `0` means success/healthy, `1` means an inspection emitted usable unhealthy state, and `2` means command or operational failure.

## Installation Boundary

The canonical artifact is `cnftctl-VERSION-debian13-amd64.tar.gz`, a complete Debian 13 amd64 bundle. Its verifier checks manifest identity, target metadata, version syntax, and `SHA256SUMS`. Installation atomically replaces the executable, installs fixed systemd units and recovery assets, reloads systemd, and enables reconciliation plus the DDNS timer without activating firewall policy. Upgrade and uninstall are blocked while pending transactions exist, and uninstall additionally checks for active `inet hostfw`.

This section states the approved architecture contract, not release evidence. **Production status is NOT READY** until the canonical artifact passes the exact Debian 13 amd64 checks and evidence gate in `docs/manual-validation.md`; code changes alone do not satisfy that gate.

## Non-Goals

- Support beyond Debian 13 amd64.
- Arbitrary nftables management or deletion of foreign tables.
- Automatic Docker restart or container management.
- DNS-provider mutation.
- Rollback timeout customization or bypass.
- Automatic migration of incompatible desired config or presets.
- A web preset builder.
