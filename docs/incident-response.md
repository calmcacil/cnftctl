# Incident Response Runbooks

Preserve console access and collect evidence before remediation:

```sh
sudo cnftctl status --output json --detail
sudo cnftctl transactions list --output json --detail
sudo nft list ruleset
systemctl list-units 'cnftctl*' --all
journalctl -u cnftctl-firewall.service -u cnftctl-reconcile.service --since -1h
```

Store evidence securely; detailed reports can contain network identifiers.

## Pending Apply Or Lost SSH Session

1. Do not stop the rollback timer.
2. Reconnect through console or a second known-good path.
3. Identify the transaction with `cnftctl transactions list --detail`.
4. If candidate access is verified, run `cnftctl confirm ID` before the deadline.
5. Otherwise run `cnftctl rollback ID`, or wait for the timer.
6. Verify the prior generation/table state and inspect the rollback journal.

For a first install, expected rollback is absence of `inet hostfw`; for an update, expected rollback is the recorded previous generation.

## Rollback Timer Inactive

Treat an inactive timer for a pending transaction as critical. From console:

```sh
sudo cnftctl rollback TRANSACTION_ID
sudo cnftctl reconcile
sudo cnftctl doctor --detail
```

Do not confirm until the cause is known. Inspect the timer/service unit and journal, package integrity, disk space, permissions, and systemd health.

## Boot Failure Or Wrong Generation

1. Enter rescue mode or use the provider console.
2. Inspect `/var/lib/cnftctl/active`, transaction states, and generation manifests without editing them.
3. Run `cnftctl reconcile` to process unconfirmed transactions.
4. Resolve the active symlink, verify that its basename is exactly 64 lowercase hexadecimal characters, verify the immutable generation files against its manifest, and run `nft -c -f /var/lib/cnftctl/active/firewall.nft` against those exact active-generation bytes. Do not substitute validation of mutable desired config.
5. Restart `cnftctl-firewall.service` only after active-generation integrity and exact-byte nft validation pass.
6. If immediate network recovery is required and no transaction can be safely restored, delete only `table inet hostfw` with `nft delete table inet hostfw`; never use `flush ruleset`.
7. Keep delivery and durable state for forensics and repair before rebooting again.

## Desired/Active Drift

Drift normally means desired config was edited but not applied/confirmed. Review `cnftctl plan`; either apply it through the normal transaction or restore desired config from an approved backup. Never edit files inside a generation.

## DDNS Failure

1. Check `cnftctl ddns status --detail` and the DDNS refresh journal.
2. Verify DNS resolution and system time without exposing private hostnames in public tickets.
3. Run one manual `cnftctl ddns refresh`.
4. If trust cannot be restored before TTL expiry, use an already-approved static entry or trusted console path; do not broaden a prefix casually.
5. Disable DDNS through desired config and apply/confirm if the feature must be withdrawn.

## Docker Exposure Or Outage

1. Capture cnftctl and Docker tables before changing either system.
2. Close the affected tuple in desired config, apply, verify, and confirm.
3. Do not restart Docker as part of cnftctl remediation unless separately approved.
4. If foreign or Docker tables disappeared, treat it as a separate nftables owner incident; cnftctl rollback must target only `inet hostfw`.
5. Validate both host and container services because one `open_ports` tuple can expose both.

## Safe Policy Deactivation For Uninstall

There is no normal CLI command that deactivates a confirmed policy. If uninstall is required, use console access, record the active generation, ensure no transaction is pending, and explicitly run:

```sh
sudo nft delete table inet hostfw
sudo apt remove cnftctl
```

This is an incident/retirement operation, not a rollback bypass. It removes only the app-owned table. Preserve `/etc/cnftctl` and `/var/lib/cnftctl` until retention and forensic requirements are satisfied.
