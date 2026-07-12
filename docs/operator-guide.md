# Operator Guide

## Install And Upgrade

Verify and install only a complete release bundle:

```sh
./scripts/verify-bundle .
sudo ./install.sh
```

Installation deploys `/usr/bin/cnftctl`, fixed units under `/usr/lib/systemd/system`, and delivery metadata under `/var/lib/cnftctl`. It enables boot reconciliation and the DDNS timer unit but does not activate firewall policy. At boot and after policy transitions, reconciliation derives whether the timer runs from the selected active generation, never from unapplied desired config.

The upgrade contract is to verify the new bundle and run its `install.sh`; it must refuse while `transactions list` reports any pending transaction and preserve desired configuration, generations, ownership, and active selection. Use upgrade in production only after the exact candidate artifact passes the upgrade section of `docs/manual-validation.md`.

## Desired Policy

Create desired state with `init`, or inspect `/etc/cnftctl/config.yaml` using `cnftctl config show`. The file is strict schema version `1`. Unknown fields fail validation.

```sh
sudo cnftctl init --wan-interface eth0 --yes
sudo cnftctl open tcp 443 --comment "public HTTPS"
sudo cnftctl whitelist add 203.0.113.10 --comment "example administrator"
sudo cnftctl validate
sudo cnftctl plan
```

Desired config is mutable operator intent, not a staging copy of live rules. Desired edits are inert until `apply` creates and selects an immutable generation. `ports list` explicitly warns that desired and active policy may differ.

## Apply, Confirm, And Roll Back

```sh
sudo cnftctl apply
sudo cnftctl transactions list --detail
sudo cnftctl confirm TRANSACTION_ID
```

Verify access before confirming. Omitting a transaction ID is accepted only when exactly one transaction is pending. To reject a candidate immediately:

```sh
sudo cnftctl rollback TRANSACTION_ID
```

Do not stop rollback units to extend the review window. Prepare and validate another desired generation instead.
The rollback deadline is fixed at 120 seconds; there is no timeout option or supported bypass.

## Reboot Behavior

The active symlink selects the generation loaded by `cnftctl-firewall.service` before normal networking. `cnftctl-reconcile.service` processes unconfirmed durable transactions on boot and reconciles DDNS timer activity from the resulting selected generation. A confirmed selection survives reboot; an unconfirmed selection returns to the prior generation, or removes `inet hostfw` when no prior generation exists.

## SSH Hardening

SSH modes are `open`, `whitelist-only`, and `whitelist-rate-limit`. Configure static and/or DDNS coverage before hardening, then apply through a console-capable maintenance window.

If the current SSH client is not covered, apply refuses. The emergency override is explicit and audited:

```sh
sudo cnftctl apply \
  --acknowledge-ssh-lockout-risk \
  --reason "console recovery available during approved maintenance"
```

The override always requires a non-empty reason, including from an interactive terminal. It acknowledges risk only; it does not disable validation or rollback.

## DDNS

```sh
sudo cnftctl ddns add home.example.com
sudo cnftctl ddns enable
sudo cnftctl apply
sudo cnftctl confirm TRANSACTION_ID
sudo cnftctl ddns refresh
sudo cnftctl ddns status --output json
```

Hostnames are part of the SSH trust boundary. A records are exact IPv4 entries. AAAA records derive `/56` prefixes by default; choose `/64` only to trust one LAN. Runtime elements expire at the configured TTL if refresh stops. `status`/`doctor` report resolution, runtime-set, and freshness failures; `--detail` reveals hostnames and addresses.

## Docker

Enable strict gating only after reviewing exposure:

```sh
sudo cnftctl feature enable docker
sudo cnftctl docker status
sudo cnftctl docker backend plan
sudo cnftctl apply
```

Every WAN-reachable published port must also be in `open_ports`. Backend writes require `--yes`, create a backup, and do not restart Docker. Schedule a separate Docker restart and validate Docker's own tables and container reachability afterward.

## Status And Doctor

```sh
sudo cnftctl status
sudo cnftctl doctor --output json
sudo cnftctl status --detail
```

`doctor` currently aliases the full status inspection. Checks cover target platform, installed/rendered assets, `inet hostfw`, exact nft validation, desired config, generation and ownership integrity, desired/active drift, transaction timers, and DDNS state. Exit `1` means the report is usable but unhealthy; exit `2` means execution failed.

## Logging

CLI output goes to its caller. systemd records boot loads, rollback, reconciliation, and DDNS refresh:

```sh
journalctl -u cnftctl-firewall.service --since today
journalctl -u cnftctl-reconcile.service --since today
journalctl -u 'cnftctl-rollback@*.timer' -u 'cnftctl-rollback@*.service' --since today
journalctl -u cnftctl-ddns-refresh.service --since today
```

Capture `status --output json` without `--detail` for routine support. Treat detailed output and transaction files as sensitive because they can include addresses, hostnames, SSH connection context, and override reasons.

## Uninstall

Uninstall must refuse while `transactions list` reports pending transactions or `inet hostfw` is active, because removing rollback and boot assets in that state is unsafe. After the exact artifact passes manual validation, first follow the policy-deactivation incident procedure with console access, verify the managed table is absent, and run from that verified bundle:

```sh
sudo ./uninstall.sh
```

Uninstall removes delivery assets and preserves configuration. It does not promise to erase `/etc/cnftctl`, generations, or audit evidence.

## Recovery

Use `cnftctl status --detail`, pending transaction state, unit status, and journals before changing files. Prefer `cnftctl rollback ID` for a pending transaction and `cnftctl reconcile` for all unconfirmed transactions. Validate the selected active generation and use the recovery assets from the same verified bundle. See `docs/incident-response.md`.
