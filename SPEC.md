# cnftctl Implemented Architecture

## Scope And Invariants

`cnftctl` exclusively owns and manages the app-owned `inet hostfw` nftables table on Debian 13 amd64 and experimental arm64. It does not manage arbitrary rules, Docker containers, DNS provider records, routing policy, or other nftables tables. An unknown pre-existing `inet hostfw` table is rejected; adoption is limited to the known reference layout. It never issues `flush ruleset`.

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
| Pending transaction | `/run/cnftctl/` | Runtime state for the currently pending activation. |
| Runtime lock | `/run/cnftctl/apply.lock` | Serializes apply, rollback, reconciliation, and DDNS set replacement. |

Desired config is the only mutable source of operator intent. Desired edits do not mutate a generation or live rules. Active policy comes only from an immutable content-addressed generation selected through the `cnftctl`-owned `/etc/nftables.conf` bootstrap. A generation hash is computed from a semantic manifest; embedded generation paths are normalized before hashing. Existing generations are hash-verified before reuse.

## Activation Transaction

The only normal active-policy transition is `cnftctl apply`:

1. Load and validate desired config.
2. Render final-shaped generation files.
3. Validate the exact final candidate bytes with `nft -c`, using the candidate directory as nft's explicit include path.
4. Acquire the runtime lock and reject concurrent or pending applies.
5. Verify installed delivery assets and existing `inet hostfw` ownership.
6. Durably write generation files, manifest, and prepared transaction state, then create pending runtime state.
7. Arm and verify the installed `cnftctl-rollback@ID.service` and timer before changing the selector.
8. Atomically update `/var/lib/cnftctl/active`.
9. Restart the dedicated `cnftctl-firewall.service`, which activates the selected generation with its immutable directory as nft's explicit include path.
10. Record ownership and reconcile the DDNS refresh timer with generation intent.
11. Require confirmation before the fixed 120-second deadline.

`confirm` durably marks the transaction confirmed and stops its rollback timer. On timeout, a fresh install deletes only `table inet hostfw` and removes the active selector; an update selects and loads the previous generation. Rollback also restores the prior generation's DDNS timer intent. Activation failures invoke the same recovery path.

The installer enables and starts `cnftctl-reconcile.service`. The oneshot remains active after its successful initial reconciliation, so the firewall unit's required dependency cannot rerun reconciliation in the middle of an apply transaction. Both units are installed into `multi-user.target`; the firewall unit pulls in `network-pre.target`, while ordering and the required dependency prevent firewall activation until reconciliation succeeds. Boot reconciliation accepts an absent managed table only when the durable selector and ownership still match the unconfirmed transaction, then restores the prior generation before boot firewall activation.

Rollback is systemd-owned through installed service and timer templates, not attached to the initiating process. `cnftctl-reconcile.service` runs during boot, treats every unconfirmed durable transaction as failed, and restores the last-known-good generation. The firewall service is a dedicated oneshot unit ordered before `network.target`; it loads the selected immutable generation directly rather than using the distribution `nftables.service`.

## SSH Safety Override

SSH remains open by default. Selecting whitelist-only or whitelist-rate-limit hardening is explicit and explains the lockout risk. For hardened modes, apply checks the client address from `SSH_CONNECTION` or `SSH_CLIENT` against static allowlists, current DDNS resolution, and a configured trusted-interface server address. An uncovered session is rejected by default.

`--acknowledge-ssh-lockout-risk` permits an explicit override only when accompanied by a non-empty `--reason`, regardless of whether invocation is interactive. Acknowledgement, reason, CLI source, and connection context are stored in transaction state with bounded lengths. This override does not disable rollback.

## DDNS Policy

DDNS is disabled by default. Enabling it requires at least one syntactically valid hostname. Every configured hostname must resolve to at least one usable address before an update can proceed. A records produce exact IPv4 set elements. AAAA addresses are masked to `/56` by default; `/64` is allowed only when explicitly selected for single-LAN trust. Runtime elements carry the configured TTL.

Refresh resolves all configured names before replacing both nftables sets in one atomic nftables batch under the firewall lock. Any DNS or batch failure preserves all previous runtime entries. Attempt metadata is written even when refresh fails, allowing `status` and `doctor` to report stale or failed runtime state. The installed `cnftctl-ddns-refresh.timer` is enabled for boot. Reconciliation starts or stops its runtime activity according to the selected generation's DDNS intent after activation, rollback, and boot; desired config alone has no authority over the timer.

## Docker Policy

Docker support is opt-in and strict: WAN-to-Docker forwarding is denied unless the protocol/port tuple exists in `open_ports`. Every open port represents public WAN exposure for both host services and Docker-published services when Docker gating is enabled. IPv4 DNAT compares the original public destination port. IPv6 DNAT and routed container traffic use the applicable destination-port gate. The forwarding chain otherwise preserves Docker's own forwarding behavior and unrelated Docker tables are untouched.

Daemon configuration inspection and writing are separate from firewall apply. Live backend plans and writes validate the exact proposed JSON with the installed Docker daemon and refuse unsupported directives without changing the file. Backend writes preserve valid JSON, create a timestamped backup, require `--yes`, and never restart Docker. Alternate-root previews remain offline and therefore cannot establish installed-daemon compatibility.

## Configuration And Commands

Config schema version is `1`. Unknown fields and versions are rejected. Ports support TCP/UDP single ports and ranges. Static whitelist entries are structured values with optional comments. Docker has only `enabled` and `interfaces`; there is no allow-published-by-default mode.

Policy-mutating commands change mutable desired operator intent only: `init`, `open`, `close`, `whitelist`, `ddns` config commands, `ssh-harden`, `feature`, and `adopt reference`. Inspection and lifecycle commands include `validate`, `plan`, `apply`, `confirm`, `rollback`, `reconcile`, `transactions list`, `status`, and `doctor`. `transactions list` reports pending transactions rather than durable history. `doctor` currently runs the same comprehensive inspection as `status`.

Presets are untrusted, versioned JSON wrappers around the same strict config. They may prefill desired state but cannot activate it or bypass validation, risk explanations, local confirmation, SSH checks, or rollback.

## Reports And Exit Codes

Inspection reports support text and JSON. JSON schema `cnftctl.report.v1` contains `command`, conservative overall `state`, `checks`, and optional `data`. States are `ok`, `absent`, `pending`, `degraded`, `failed`, `unknown`, `unsupported`, and `not_applicable`. Sensitive addresses and hostnames are withheld unless `--detail` is supplied where implemented.

Exit `0` means success/healthy, `1` means an inspection emitted usable unhealthy state, and `2` means command or operational failure.

## Installation Boundary

Release artifacts are native Debian 13 packages named `cnftctl_VERSION_ARCH.deb` for `amd64` and `arm64`. Builders and verifiers require an explicit supported architecture; control metadata, manifest, pre-install guard, and ELF machine must agree. Installation places the executable, fixed systemd units, integrity inventory, documentation, and recovery assets, reloads systemd, and enables only reconciliation without activating firewall policy. Arm64 installation emits an experimental-risk warning. Upgrade and removal are blocked while transaction history is unresolved or unsafe, and removal additionally checks for active `inet hostfw`. Removal and purge preserve operator configuration, immutable generations, and transaction audit state.

This section states the approved architecture contract, not release evidence. A version is production-supported only after its exact amd64 package passes the Debian 13 amd64 evidence gate in `docs/manual-validation.md`; code changes and native CI alone do not satisfy that gate. Arm64 is experimental until an exact package completes the same full live checklist on a suitable disposable arm64 host.

## Non-Goals

- Production support beyond Debian 13 amd64; arm64 is experimental only.
- Arbitrary nftables management or deletion of foreign tables.
- Automatic Docker restart or container management.
- DNS-provider mutation.
- Rollback timeout customization or bypass.
- Automatic migration of incompatible desired config or presets.
- A web preset builder.
