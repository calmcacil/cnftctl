# Operator Guide

## Install And Upgrade

Download the Debian package, checksums, and provenance from the same release. Verify and install the package:

```sh
sha256sum --ignore-missing --check release-checksums.txt
sha256sum --check release-checksums.txt
gh attestation verify "cnftctl_VERSION_$(dpkg --print-architecture).deb" --repo calmcacil/cnftctl
sudo apt install "./cnftctl_VERSION_$(dpkg --print-architecture).deb"
```

Use `sbom_amd64.spdx.json` with the production-supported amd64 package or `sbom_arm64.spdx.json` with the experimental arm64 package. Arm64 is unvalidated on a disposable live host, is not production-supported, and is used at your own risk; its installer repeats this warning.

Installation deploys `/usr/bin/cnftctl`, fixed units under `/usr/lib/systemd/system`, and delivery metadata under `/var/lib/cnftctl`. It enables only boot reconciliation and does not activate firewall policy or DDNS. At boot and after policy transitions, reconciliation derives whether the DDNS timer runs from the selected active generation, never from unapplied desired config.

The upgrade contract is to verify the new package and install it with `apt`; package pre-installation must refuse unresolved or unsafe transaction history and preserve desired configuration, generations, ownership, and active selection. Use an upgrade in production only after the exact candidate package passes the upgrade section of `docs/manual-validation.md`.

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

Every WAN-reachable published port must also be in `open_ports`. On the live host, backend plans and writes first ask the installed Docker daemon to validate the exact proposed JSON; unsupported versions are refused without changing the file. Backend writes require `--yes`, create a backup, and do not restart Docker. Schedule a separate Docker restart and validate Docker's own tables and container reachability afterward. An alternate-root preview cannot validate compatibility with the target host's Docker daemon.

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

Removal must refuse while transaction history is unresolved or unsafe, or while `inet hostfw` is active, because removing rollback and boot assets in that state is unsafe. First follow the policy-deactivation incident procedure with console access, verify the managed table is absent, and run:

```sh
sudo apt remove cnftctl
```

Both `apt remove` and `apt purge` remove delivery assets while preserving `/etc/cnftctl`, immutable generations, and audit evidence. Destructive state cleanup is a separate explicit operator action.

## Recovery

Use `cnftctl status --detail`, pending transaction state, unit status, and journals before changing files. Prefer `cnftctl rollback ID` for a pending transaction and `cnftctl reconcile` for all unconfirmed transactions. Validate the selected active generation and use the recovery assets from the same verified package. See `docs/incident-response.md`.
