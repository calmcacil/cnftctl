# How To Uninstall cnftctl

Uninstallation is deliberately more involved when a confirmed policy is
active. Removing the package first would remove the boot, rollback, and
recovery assets while `inet hostfw` still controls connectivity, so package
removal refuses that state.

Both `apt remove` and `apt purge` preserve `/etc/cnftctl` and
`/var/lib/cnftctl`. This retains desired configuration, immutable generations,
transaction audit state, and evidence. Purge is not a shortcut for deleting
firewall state.

## If No Policy Has Ever Been Activated

Verify that no transaction is pending and that the managed table is absent:

```sh
sudo cnftctl transactions list
if sudo nft list table inet hostfw >/dev/null 2>&1; then
  echo "inet hostfw is active; use the active-policy procedure below"
else
  echo "inet hostfw is absent"
fi
```

If there are no pending transactions and the table is absent:

```sh
sudo apt remove cnftctl
```

## If A Policy Is Active

This is a retirement/incident operation, not a normal policy change. There is
no CLI command that deactivates a confirmed generation because normal live
changes must retain rollback protection. Schedule a maintenance window and use
tested console, rescue, IPMI, or cloud serial-console access.

1. Capture the current state and keep it securely:

   ```sh
   sudo cnftctl status --output json --detail
   sudo cnftctl transactions list --output json --detail
   sudo nft list ruleset
   ```

2. Resolve every pending transaction through the supported workflow. Confirm
   only a policy you have verified; otherwise roll it back:

   ```sh
   sudo cnftctl transactions list --detail
   sudo cnftctl rollback TRANSACTION_ID
   sudo cnftctl reconcile
   sudo cnftctl transactions list
   ```

3. From the independent recovery path, delete only cnftctl's application-owned
   table:

   ```sh
   sudo nft delete table inet hostfw
   ```

   Never use `flush ruleset`; it can destroy Docker and other owners' tables.

4. Verify the table is absent and other nftables owners remain:

   ```sh
   if sudo nft list table inet hostfw >/dev/null 2>&1; then
     echo "refusing removal: inet hostfw still exists" >&2
     exit 1
   fi
   sudo nft list ruleset
   ```

5. Remove the package and check systemd:

   ```sh
   sudo apt remove cnftctl
   systemctl list-unit-files 'cnftctl*'
   systemctl --failed
   ```

Package removal disables cnftctl's firewall, reconciliation, and DDNS timer
units, removes delivery assets, and reloads systemd. It does not restart
Docker.

## Preserved State

Inspect retained state before deciding whether it may be destroyed:

```sh
sudo find /etc/cnftctl /var/lib/cnftctl -xdev -maxdepth 3 -print
```

These directories may contain sensitive hostnames, network ranges, SSH
connection context, override reasons, and audit records. Archive them with
appropriate permissions if they are needed for recovery or forensics.

Deleting preserved state is intentionally outside the package lifecycle. Only
after retention, rollback, and forensic needs have been reviewed may an
operator remove those directories manually. That destructive step cannot be
undone and is not required to uninstall cnftctl.

## If Removal Is Refused

- `managed policy inet hostfw is active`: follow the active-policy procedure;
  do not bypass the guard.
- `transaction history is corrupt or unresolved`: keep the package and its
  recovery assets installed, inspect `transactions list --detail`, transaction
  files, and journals, and follow [Incident Response](incident-response.md).
- systemd disable failure: repair systemd and retry; do not manually delete
  package files.
