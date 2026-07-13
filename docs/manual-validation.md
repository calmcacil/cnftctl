# Exact Artifact Manual Validation

Execute this checklist against the exact archive proposed for release, on disposable Debian 13 amd64 hosts with console access. Record the artifact filename, SHA-256, commit, tester, UTC timestamps, host image, nftables version, systemd version, Docker version when used, commands, outputs, and pass/fail evidence in the release issue.

The canonical archive name is `cnftctl-VERSION-debian13-amd64.tar.gz`. **Production status is NOT READY** until this checklist passes for those exact bytes; architecture documentation and code-test results are not substitutes.

Use `docs/production-readiness.md` as the release gate and record results in `docs/validation-record.md`. This file supplies the executable host steps; all three documents must refer to the same artifact SHA-256.

Use two clean validation phases: `HOST_A` without Docker and `HOST_B` with a disposable Docker installation. Separate VMs are preferred; one disposable VM may be reused only after the HOST_A approved uninstall completes, preserved cnftctl state is archived and purged, absence of the managed table is verified, and Docker is installed afterward. Keep an independent SSH session and console open throughout active-policy tests.

## Artifact Identity

```sh
export ARTIFACT=cnftctl-VERSION-debian13-amd64.tar.gz
sha256sum "$ARTIFACT"
rm -rf /tmp/cnftctl-artifact
mkdir /tmp/cnftctl-artifact
tar -xzf "$ARTIFACT" -C /tmp/cnftctl-artifact --strip-components=1
cd /tmp/cnftctl-artifact
./scripts/verify-bundle .
sed -n '1,20p' manifest
sha256sum -c SHA256SUMS
```

- [ ] Archive checksum matches published release evidence.
- [ ] Bundle verification succeeds without network access.
- [ ] Manifest says `format=1`, `product=cnftctl`, `architecture=amd64`, `os=debian-13`, and the intended version.
- [ ] `SHA256SUMS` covers the delivered payload and no unexpected executable or secret is present.

## Staged Install Contract

```sh
rm -rf /tmp/cnftctl-root
mkdir /tmp/cnftctl-root
CNFTCTL_INSTALL_ROOT=/tmp/cnftctl-root CNFTCTL_BUNDLE_ARCH=amd64 ./install.sh
test -x /tmp/cnftctl-root/usr/bin/cnftctl
test -f /tmp/cnftctl-root/var/lib/cnftctl/delivery/manifest
CNFTCTL_INSTALL_ROOT=/tmp/cnftctl-root ./uninstall.sh --force-inactive
test ! -e /tmp/cnftctl-root/usr/bin/cnftctl
```

- [ ] Staged install and uninstall succeed and leave no delivery binary.
- [ ] Staged uninstall reports that configuration is preserved.

## First Install On HOST_A

```sh
sudo ./install.sh
/usr/bin/cnftctl --version
systemctl is-enabled cnftctl-reconcile.service
! systemctl is-enabled cnftctl-firewall.service
sudo cnftctl init --dry-run --wan-interface eth0
sudo cnftctl init --wan-interface eth0 --yes
sudo cnftctl validate --output json | tee /tmp/validate.json
sudo cnftctl plan --output json | tee /tmp/plan.json
sudo nft list tables | tee /tmp/tables-before-first-apply
```

- [ ] Install rejects a non-Debian-13 or non-amd64 test root/host.
- [ ] Install does not create or load `inet hostfw`.
- [ ] Install enables and starts boot reconciliation but leaves the firewall service disabled until a successful apply.
- [ ] `init --dry-run` writes nothing.
- [ ] Desired config is mode `0600`, schema version `1`, SSH mode `open`, and has no open public ports.
- [ ] JSON parses and has schema `cnftctl.report.v1`.

## First Install Timeout Deletes The Table

```sh
sudo cnftctl apply | tee /tmp/first-apply.txt
TX=$(sed -n 's/^applied transaction \([0-9a-f]\{32\}\) .*/\1/p' /tmp/first-apply.txt)
test "${#TX}" -eq 32
case "$TX" in *[!0-9a-f]*) exit 1 ;; esac
sudo nft list table inet hostfw
systemctl is-active "cnftctl-rollback@$TX.timer"
deadline=$(( $(date +%s) + 150 ))
while sudo nft list table inet hostfw >/dev/null 2>&1; do
    test "$(date +%s)" -lt "$deadline" || { echo "rollback deadline exceeded" >&2; exit 1; }
    sleep 1
done
! sudo nft list table inet hostfw
test ! -e /var/lib/cnftctl/active
sudo cnftctl transactions list
```

- [ ] Apply prints transaction ID, generation, deadline, and exact confirm instruction.
- [ ] Timer is active before timeout.
- [ ] At timeout only `inet hostfw` is deleted; tables captured before apply still exist.
- [ ] Transaction state says `fresh_install: true`, `rolled_back: true`, and `phase: rolled-back`.

## Confirmed Generation And Prior-Generation Rollback

```sh
sudo cnftctl apply | tee /tmp/confirmed-apply.txt
TX=$(sed -n 's/^applied transaction \([0-9a-f]\{32\}\) .*/\1/p' /tmp/confirmed-apply.txt)
test "${#TX}" -eq 32
sudo cnftctl confirm "$TX"
GEN1=$(basename "$(readlink /var/lib/cnftctl/active)")
sudo nft list table inet hostfw > /tmp/gen1.nft
sudo cnftctl open tcp 8443 --comment "validation only"
sudo cnftctl apply | tee /tmp/update-apply.txt
TX2=$(sed -n 's/^applied transaction \([0-9a-f]\{32\}\) .*/\1/p' /tmp/update-apply.txt)
test "${#TX2}" -eq 32
GEN2=$(basename "$(readlink /var/lib/cnftctl/active)")
test "$GEN1" != "$GEN2"
deadline=$(( $(date +%s) + 150 ))
while test "$(basename "$(readlink /var/lib/cnftctl/active)")" != "$GEN1"; do
    test "$(date +%s)" -lt "$deadline" || { echo "rollback deadline exceeded" >&2; exit 1; }
    sleep 1
done
test "$(basename "$(readlink /var/lib/cnftctl/active)")" = "$GEN1"
sudo nft list table inet hostfw > /tmp/after-update-timeout.nft
diff -u /tmp/gen1.nft /tmp/after-update-timeout.nft
```

- [ ] Confirmation stops the timer and generation remains active after 125 seconds.
- [ ] A later timeout restores the exact prior generation and marks the transaction rolled back.
- [ ] Generation directories and manifests are read-only and hashes match their manifest entries.

## Session Death And Reboot

```sh
# In an expendable SSH session:
sudo cnftctl open udp 8443 --comment "session-death validation"
sudo cnftctl apply
# Immediately terminate the SSH client without confirming, then reconnect after 125 seconds.
sudo cnftctl status --detail

# Start another unconfirmed apply, then reboot immediately from the console.
sudo cnftctl apply
sudo reboot
# After boot:
systemctl status cnftctl-reconcile.service --no-pager
sudo cnftctl transactions list
sudo cnftctl status --detail
```

- [ ] Killing the invoking shell does not cancel rollback.
- [ ] Boot reconciliation rolls back the unconfirmed durable transaction.
- [ ] The previously confirmed generation is active after reboot.
- [ ] `cnftctl-firewall.service` loads the selected generation during boot.

## SSH Coverage And Override

From SSH, configure a hardened desired policy that does not cover the current client.

```sh
sudo cnftctl whitelist add 203.0.113.10 --comment "non-matching documentation address"
sudo cnftctl ssh-harden whitelist-only
! sudo cnftctl apply
! sudo cnftctl apply --acknowledge-ssh-lockout-risk
sudo cnftctl apply --acknowledge-ssh-lockout-risk --reason "manual validation with console recovery" | tee /tmp/ssh-override.txt
TX=$(sed -n 's/^applied transaction \([0-9a-f]\{32\}\) .*/\1/p' /tmp/ssh-override.txt)
test "${#TX}" -eq 32
sudo grep -q 'manual validation with console recovery' "/var/lib/cnftctl/transactions/$TX/state.json"
sudo cnftctl rollback "$TX"
```

- [ ] Uncovered SSH apply fails by default.
- [ ] Override without a reason fails from both an interactive terminal and noninteractive invocation.
- [ ] Explicit override succeeds, is audited, and does not bypass rollback.

## DDNS

Use a tester-controlled hostname whose A and AAAA records are safe for the disposable environment; do not commit it to evidence. Configure it locally, enable DDNS, apply, and confirm.

```sh
sudo cnftctl ddns add VALIDATION_HOSTNAME
sudo cnftctl ddns enable
sudo cnftctl apply | tee /tmp/ddns-apply.txt
TX=$(sed -n 's/^applied transaction \([0-9a-f]\{32\}\) .*/\1/p' /tmp/ddns-apply.txt)
test "${#TX}" -eq 32
sudo cnftctl confirm "$TX"
sudo cnftctl ddns refresh
sudo cnftctl ddns status --output json --detail | tee /tmp/ddns.json
sudo nft list set inet hostfw ddns_whitelist_v4
sudo nft list set inet hostfw ddns_whitelist_v6
systemctl is-enabled cnftctl-ddns-refresh.timer
systemctl is-active cnftctl-ddns-refresh.timer
```

- [ ] A results are exact IPv4 elements with timeouts.
- [ ] AAAA results are masked to `/56`; repeat after selecting `/64` and verify `/64`.
- [ ] Failed resolution does not partially replace sets, records failure metadata, and eventually reports stale state after TTL.
- [ ] Disabling DDNS in a confirmed generation disables/stops the timer; rollback restores prior timer intent.

## Coexistence And Docker On HOST_B

Create a disposable foreign nftables table before cnftctl activation and record Docker tables before and after each operation.

```sh
sudo nft add table inet validation_foreign
sudo nft list ruleset > /tmp/rules-before-cnftctl.nft
sudo docker run -d --name cnftctl-web -p 18080:80 nginx:alpine
sudo nft list tables > /tmp/docker-tables-before
sudo ./install.sh
sudo cnftctl init --wan-interface eth0 --enable-docker --yes
sudo cnftctl apply | tee /tmp/docker-apply.txt
TX=$(sed -n 's/^applied transaction \([0-9a-f]\{32\}\) .*/\1/p' /tmp/docker-apply.txt)
test "${#TX}" -eq 32
sudo cnftctl confirm "$TX"
sudo nft list tables > /tmp/docker-tables-after
sudo nft list table inet validation_foreign
sudo cnftctl docker backend plan
```

- [ ] Foreign and Docker-owned tables survive apply, rollback, confirm, and reboot.
- [ ] WAN connection to TCP 18080 is blocked before `cnftctl open tcp 18080` is applied and confirmed.
- [ ] WAN connection succeeds after the matching public port is applied and confirmed.
- [ ] Closing and applying the port blocks it again without a Docker restart.
- [ ] IPv4 DNAT is gated by original public destination port.
- [ ] If the test host has supported IPv6 Docker DNAT/routing, matching traffic is strictly gated by destination port; otherwise record it as not exercised, not passed.
- [ ] Backend plan writes nothing.
- [ ] Live backend plan validates the exact proposal with the installed Docker daemon and refuses an unsupported backend without writing.
- [ ] Backend write refuses without `--yes`; a supported write preserves other JSON keys, creates a timestamped backup, and does not restart Docker.

## Status, Doctor, Logs, And Exit Codes

```sh
sudo cnftctl status --output json > /tmp/status.json; test "$?" -eq 0
sudo cnftctl doctor --output json > /tmp/doctor.json; test "$?" -eq 0
sudo cnftctl open tcp 9443 --comment "desired drift"
set +e
sudo cnftctl status --output json > /tmp/drift.json
test "$?" -eq 1
cnftctl --definitely-invalid
test "$?" -eq 2
set -e
journalctl -u cnftctl-firewall.service --no-pager
journalctl -u cnftctl-reconcile.service --no-pager
journalctl -u 'cnftctl-rollback@*.service' --no-pager
journalctl -u cnftctl-ddns-refresh.service --no-pager
```

- [ ] Reports identify platform, desired/active drift, generation integrity, ownership, nft validation, pending transactions/timers, and DDNS health.
- [ ] Exit codes are exactly healthy `0`, unhealthy inspection `1`, and command failure `2`.
- [ ] Journals contain actionable unit/transaction outcomes and no secrets.

## Upgrade And Uninstall

Install the previous released bundle, create and confirm a generation, then install the candidate bundle.

```sh
sudo ./install.sh
sudo cnftctl status
# From the candidate bundle:
sudo ./install.sh
sudo cnftctl status
sudo nft list table inet hostfw
! sudo ./uninstall.sh
sudo cnftctl rollback 2>/dev/null || true
```

- [ ] Upgrade preserves desired config, generations, ownership, and active policy.
- [ ] Upgrade is blocked while any durable transaction is unresolved or its state is unsafe or corrupt.
- [ ] Uninstall is blocked while `inet hostfw` is active and while transaction state is unresolved, unsafe, or corrupt; valid terminal audit history is preserved and accepted.
- [ ] After intentionally removing the managed table through the approved incident procedure and resolving transaction state, `sudo ./uninstall.sh` removes delivery assets, preserves `/etc/cnftctl/config.yaml`, and reloads systemd.
- [ ] Upgrade and uninstall consume the current pending-transaction contract; any mismatch or unsafe acceptance fails the artifact and must not be waived or edited in place.

## Release Gate

- [ ] `sh ./scripts/check.sh`, bundle build, staged validation, and delivery-asset verification pass for the tagged commit.
- [ ] All checklist evidence is attached to the release issue and references the exact artifact SHA-256.
- [ ] CI provenance, checksums, dependency/license review, known limitations, and approver sign-off are recorded.
- [ ] If release workflows are activated, both disabled workflow files were moved together to the exact `.github/workflows/release-build.yml` and `.github/workflows/release-promote.yml` paths, and promotion accepted the build workflow's attestation identity at the tagged commit.
- [ ] Documentation and examples pass the repository sanitization searches.
