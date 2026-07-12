#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
command -v systemd-analyze >/dev/null 2>&1 || { echo "systemd-analyze is required" >&2; exit 1; }
for script in "$root/scripts/check.sh" "$root/scripts/build-bundle.sh" "$root/scripts/validate-staged.sh" "$root/scripts/verify-delivery-assets.sh" "$root/packaging/test-bundle.sh" "$root/packaging/bundle/install.sh" "$root/packaging/bundle/uninstall.sh" "$root/packaging/bundle/scripts/cnftctl-recover" "$root/packaging/bundle/scripts/inspect-transaction" "$root/packaging/bundle/scripts/verify-bundle"; do
    sh -n "$script"
done

for unit in "$root"/deploy/systemd/*; do
    systemd-analyze verify "$unit" 2>&1 | while IFS= read -r line; do
        case $line in
            *'Command /usr/bin/cnftctl is not executable: No such file or directory'*) ;;
            *'Command /usr/bin/nft is not executable: No such file or directory'*) ;;
            *'Command /usr/lib/cnftctl/cnftctl-recover is not executable: No such file or directory'*) ;;
            '') ;;
            *) echo "$line" >&2; exit 1 ;;
        esac
    done
done

grep -q '^ExecStart=/usr/bin/nft -f /var/lib/cnftctl/active/firewall.nft$' "$root/deploy/systemd/cnftctl-firewall.service"
grep -q '^ExecStart=/usr/bin/cnftctl rollback %i$' "$root/deploy/systemd/cnftctl-rollback@.service"
grep -q '^ExecStart=/usr/bin/cnftctl reconcile$' "$root/deploy/systemd/cnftctl-reconcile.service"
if grep -Eq 'systemctl (enable|start|enable --now|start --no-block).*cnftctl-firewall' "$root/packaging/bundle/install.sh"; then
    echo "installer must not activate the firewall unit" >&2
    exit 1
fi
grep -q 'name: Release Candidate Build$' "$root/.github/workflows-disabled/release-build.yml"
grep -q 'actions/workflows/release-build.yml@\$GITHUB_SHA' "$root/.github/workflows-disabled/release-promote.yml"
grep -q 'cnftctl-${{ inputs.version }}-debian13-amd64.tar.gz' "$root/.github/workflows-disabled/release-build.yml"

sh "$root/packaging/test-bundle.sh"
