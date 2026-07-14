# How To Use cnftctl

cnftctl manages only the application-owned nftables table `inet hostfw`. Its
central safety rule is that desired configuration is not live policy:

```text
edit desired config -> validate -> plan -> apply (rollback armed) -> verify -> confirm
```

Commands such as `open`, `close`, `whitelist`, `ddns`, `ssh-harden`, and
`feature` edit `/etc/cnftctl/config.yaml`. They do not change live nftables
until `apply` creates and selects an immutable, content-addressed generation.
Every real activation has a fixed 120-second dead-man rollback and must be
confirmed.

Use root for installed-state and live-policy operations. Run
`cnftctl COMMAND --help` for the flags accepted by a command.

## Global Options

| Option | Meaning |
| --- | --- |
| `-h`, `--help` | Show help for the current command path. |
| `--version` | Print the embedded version; it cannot be combined with a command. |
| `--config PATH` | Use a desired-config path instead of `/etc/cnftctl/config.yaml`. |
| `--output text\|json` | Select report output. Valid only for reporting commands listed below. |
| `--detail` | Include potentially sensitive report details. Valid only for reporting commands. |
| `--root PATH` | Inspect or preview an alternate filesystem root. This is for tests/offline previews, not a supported production-installation mode. |

`status`, `doctor`, `validate`, `plan`, `transactions list`, and `ddns status`
are reporting commands. Their JSON schema is `cnftctl.report.v1`.

## Installation And Configuration

### `cnftctl status`

Inspects platform support, installed assets, desired config, active generation
and manifest, ownership, desired/active drift, the live table, transactions,
rollback timers, and DDNS state. Use it as the broad operational overview.

```sh
sudo cnftctl status
sudo cnftctl status --output json
sudo cnftctl status --detail
```

`--detail` can reveal generation IDs, addresses, hostnames, SSH context, and
audit reasons; protect captured output.

### `cnftctl doctor`

Currently performs the same full inspection as `status`. Use the name in
health checks or troubleshooting workflows where its intent is clearer.

```sh
sudo cnftctl doctor --output json
```

### `cnftctl config show`

Prints normalized mutable desired configuration. It does not print the
currently active immutable generation, and the two may differ.

```sh
sudo cnftctl config show
```

### `cnftctl init`

Creates initial desired config from safe defaults, optionally using a preset.
It reviews the result and requires `--yes` before writing. It never activates
the firewall.

```sh
sudo cnftctl init --wan-interface eth0 --yes
sudo cnftctl init --wan-interface eth0 --dry-run
sudo cnftctl init --wan-interface eth0 \
  --trust-interface tailscale0 --enable-docker --yes
sudo cnftctl init --preset-file preset.v1.json --yes
```

Options are `--wan-interface NAME`, repeatable `--trust-interface NAME`,
`--enable-docker`, `--enable-ddns-whitelist`, `--preset VALUE`,
`--preset-file PATH`, `--dry-run`, and `-y`/`--yes`. `--preset` and
`--preset-file` are mutually exclusive. Presets are untrusted input: inspect
and explain them first, and do not let them substitute for local review.

### `cnftctl validate`

Validates the strict config schema, renders the exact candidate generation,
checks its manifest, and runs nftables syntax validation against those final
bytes. It does not write or load policy.

```sh
sudo cnftctl validate
sudo cnftctl validate --output json
```

### `cnftctl plan`

Reports the candidate generation, file changes, and whether active nftables
would change. A pending plan normally exits `1`, allowing automation to
distinguish drift from a no-op.

```sh
sudo cnftctl plan
sudo cnftctl plan --output json
```

## Activation And Transactions

### `cnftctl apply`

Validates final bytes, resolves and seeds enabled DDNS entries, checks current
SSH-session coverage, writes a durable generation and transaction, arms and
verifies the systemd rollback timer, then activates through
`cnftctl-firewall.service`. It prints the transaction ID and deadline.

```sh
sudo cnftctl apply --dry-run
sudo cnftctl apply
```

When hardened SSH would not cover the current session, apply refuses. Only
with tested recovery access may an operator use the audited acknowledgement:

```sh
sudo cnftctl apply \
  --acknowledge-ssh-lockout-risk \
  --reason "approved maintenance with console recovery"
```

The acknowledgement never disables validation or rollback. If the requested
generation is already active, apply is a no-op and creates no transaction.

### `cnftctl confirm [TRANSACTION_ID]`

Marks the candidate as accepted and cancels its rollback timer. Omit the ID
only when exactly one transaction is pending.

```sh
sudo cnftctl confirm 0123456789abcdef0123456789abcdef
```

Confirm only after testing current SSH, a second administrative connection,
required host services, DDNS behavior, and any Docker-published service.

### `cnftctl rollback [TRANSACTION_ID]`

Immediately rejects a pending candidate and restores its recorded previous
generation. For a first activation it removes only `inet hostfw`. Omit the ID
only when exactly one transaction is pending.

```sh
sudo cnftctl rollback 0123456789abcdef0123456789abcdef
```

### `cnftctl transactions list`

Lists pending transactions, their candidate generations, phases, and
deadlines. It is not a complete transaction-history browser.

```sh
sudo cnftctl transactions list --detail
sudo cnftctl transactions list --output json
```

### `cnftctl reconcile`

Rolls back all unconfirmed durable transactions and reconciles runtime state.
The boot reconciliation service uses this behavior after an interrupted apply
or reboot.

```sh
sudo cnftctl reconcile
```

Prefer `rollback ID` for one known pending transaction. Use `reconcile` during
recovery when every unconfirmed transaction must be returned to a safe state.

## Public Ports

An open-port entry is public WAN exposure for a matching host service. When
Docker gating is enabled, the same protocol/public-port tuple also permits a
matching Docker-published service.

### `cnftctl open <tcp|udp> <PORT-OR-RANGE>`

Adds an idempotent desired entry. A single port such as `443` or an inclusive
range such as `8000-8010` is accepted.

```sh
sudo cnftctl open tcp 443 --comment "public HTTPS"
sudo cnftctl open udp 41641 --comment "Tailscale direct connectivity"
sudo cnftctl open tcp 8000-8010
```

### `cnftctl close <tcp|udp> <PORT-OR-RANGE>`

Removes a desired entry. Missing entries are normally an idempotent no-op;
`--strict` turns that into an error.

```sh
sudo cnftctl close tcp 443
sudo cnftctl close udp 41641 --strict
```

### `cnftctl ports list`

Lists desired open ports and comments. The output reminds you that desired and
active policy may differ until a new apply is confirmed.

```sh
sudo cnftctl ports list
```

After any changed port command, run `validate`, `plan`, `apply`, test from the
WAN, and `confirm`.

## Static SSH Allowlist And Hardening

Static entries accept IP addresses and CIDRs. Hostnames do not belong here;
use DDNS. Documentation examples use reserved non-routable ranges.

### `cnftctl whitelist add <IP-OR-CIDR>`

Adds an idempotent desired static SSH trust entry.

```sh
sudo cnftctl whitelist add 203.0.113.10 --comment "example administrator"
sudo cnftctl whitelist add 2001:db8:1234::/64 --comment "example IPv6 LAN"
```

### `cnftctl whitelist remove <IP-OR-CIDR>`

Removes an entry. `--strict` fails if it was not configured.

```sh
sudo cnftctl whitelist remove 203.0.113.10
```

### `cnftctl whitelist list`

Lists normalized desired static prefixes.

```sh
sudo cnftctl whitelist list
```

### `cnftctl ssh-harden open`

Keeps SSH publicly reachable from WAN. This is the default and the safest
first-install mode against accidental remote lockout.

```sh
sudo cnftctl ssh-harden open
```

### `cnftctl ssh-harden whitelist-only`

Changes desired SSH exposure so only static or DDNS allowlist entries are
accepted. It refuses an obviously empty allowlist unless `--force` is supplied.
`--force` only permits writing risky desired intent; apply still performs
current-session coverage checks and always arms rollback.

```sh
sudo cnftctl ssh-harden whitelist-only
```

### `cnftctl ssh-harden whitelist-rate-limit`

Uses allowlists plus connection rate limits. On first selection it creates the
default rate limit of 6 connections per minute. It has the same `--force`
semantics and lockout risk as `whitelist-only`.

```sh
sudo cnftctl ssh-harden whitelist-rate-limit
```

Always harden SSH during a console-capable maintenance window, apply, prove
coverage from a second session, and only then confirm.

## DDNS SSH Allowlist

DDNS hostnames are part of the SSH trust boundary. A records become exact IPv4
entries. AAAA addresses become `/56` prefixes by default; `/64` is the only
alternative and should be used only when trusting one LAN prefix is intended.
Runtime entries have timeouts and expire if successful refresh stops.

### Configuration commands

```sh
sudo cnftctl ddns add home.example.com
sudo cnftctl ddns remove home.example.com
sudo cnftctl ddns remove home.example.com --strict
sudo cnftctl ddns set-ipv6-prefix-len 56
sudo cnftctl ddns enable
sudo cnftctl ddns disable
```

`add` and `remove` edit the desired hostname list; `--strict` makes a missing
remove fail. `set-ipv6-prefix-len` accepts only `56` or `64`. `enable` and
`disable` edit desired feature intent. Apply and confirm are required before
timer activity follows the new intent.

### `cnftctl ddns refresh`

Resolves all configured names, atomically replaces runtime set contents, and
records attempt/freshness metadata. It fails closed when the candidate cannot
be safely built; it does not broaden trust from partial or stale guesses.

```sh
sudo cnftctl ddns refresh
```

### `cnftctl ddns status`

Reports enabled intent, DNS resolution, configured versus runtime entry
counts, and freshness. Detailed output includes the sensitive hostnames and
addresses.

```sh
sudo cnftctl ddns status
sudo cnftctl ddns status --output json
sudo cnftctl ddns status --detail
```

### `cnftctl ddns timer status`

Shows whether `cnftctl-ddns-refresh.timer` is enabled and active. Timer state
is derived from the selected active generation after activation, rollback, and
boot—not directly from unapplied desired config.

```sh
sudo cnftctl ddns timer status
```

## Optional Features

### `cnftctl feature enable|disable docker`

Changes desired strict Docker WAN-gating intent. It does not edit
`daemon.json`, restart Docker, or activate policy.

```sh
sudo cnftctl feature enable docker
sudo cnftctl feature disable docker
```

When enabled, WAN traffic to Docker-published services is admitted only for a
matching `open_ports` tuple. That tuple also exposes a matching host service.

### `cnftctl feature enable|disable trusted-interface`

Adds or removes explicit trusted interfaces. `--interface`/`-i` is required
and repeatable.

```sh
sudo cnftctl feature enable trusted-interface --interface tailscale0
sudo cnftctl feature disable trusted-interface --interface tailscale0
```

Traffic arriving on a trusted interface receives full configured host-input
trust, including SSH. The interface is never auto-discovered. Do not use this
as a substitute for the overlay/VPN product's own authentication and ACLs.

## Docker Inspection And Backend Configuration

These commands are separate from `feature enable docker`. They inspect or
prepare Docker's own daemon configuration; cnftctl never restarts Docker.

### `cnftctl docker status`

Shows Docker's configured `firewall-backend`, or reports that it is not set.
Use `--daemon-json PATH` for a non-default config file.

```sh
sudo cnftctl docker status
```

### `cnftctl docker backend plan`

Previews setting `firewall-backend` to `nftables`, preserving other JSON keys,
and shows the timestamped backup path. On the live host it asks the installed
Docker daemon to validate the exact proposed JSON. Unsupported Docker versions
are rejected without a write.

```sh
sudo cnftctl docker backend plan
```

### `cnftctl docker backend write`

Repeats validation, creates the backup, and writes the proposed config. It
requires `--yes` and still does not restart Docker.

```sh
sudo cnftctl docker backend write --yes
```

A Docker backend migration and restart are disruptive separate operator
actions. Obtain explicit approval, schedule them, restart Docker manually, and
then validate Docker tables, bridges, host services, and container reachability.

## Reference Adoption And Presets

### `cnftctl adopt reference`

Imports supported open-port and static-whitelist values from the sanitized
legacy reference layout into new desired config. It prints warnings and a
review, requires `--yes` to write, and does not activate policy.

```sh
sudo cnftctl adopt reference --dry-run
sudo cnftctl adopt reference --yes
```

### `cnftctl preset decode <PRESET>`

Decodes a base64url JSON preset and prints its config as JSON. It does not
write desired state.

```sh
cnftctl preset decode 'BASE64URL_VALUE'
```

### `cnftctl preset validate <FILE>`

Validates a preset file's schema and values without applying it.

```sh
cnftctl preset validate preset.v1.json
```

### `cnftctl preset explain <FILE>`

Explains the preset's policy impact and risks for human review.

```sh
cnftctl preset explain preset.v1.json
```

Presets can pre-fill intent but cannot bypass config validation, risk review,
local confirmation, SSH coverage checks, or rollback.

## Reporting Output And Exit Codes

Text output is intended for operators. JSON reports use stable check IDs,
states, summaries, optional codes/details, and command-specific data. Do not
parse human-readable text in automation.

| Exit | Meaning |
| --- | --- |
| `0` | Command succeeded; a report is healthy or not applicable. |
| `1` | Inspection completed with usable output but found absent, pending, degraded, unsupported, unknown, or failed state. |
| `2` | Usage, validation, permission, I/O, or operational failure. |

An exit `1` from `plan` can simply mean changes are pending. An exit `1` from
`status` or `doctor` requires reading the individual checks. An unsupported
platform report is informational evidence, not permission to apply there.

## A Safe Everyday Workflow

```sh
# Edit desired intent with a cnftctl command.
sudo cnftctl open tcp 443 --comment "public HTTPS"

# Review inert desired state and exact rendered candidate.
sudo cnftctl config show
sudo cnftctl validate
sudo cnftctl plan

# Activate under the rollback timer.
sudo cnftctl apply
sudo cnftctl transactions list --detail

# Test access and services from independent paths, then accept.
sudo cnftctl confirm TRANSACTION_ID
sudo cnftctl status
```

For recovery and retirement procedures, see [Incident Response](incident-response.md)
and [How To Uninstall](uninstall.md).
