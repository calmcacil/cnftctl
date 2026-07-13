#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
command -v systemd-analyze >/dev/null 2>&1 || { echo "systemd-analyze is required" >&2; exit 1; }
for script in "$root/scripts/check.sh" "$root/scripts/build-bundle.sh" "$root/scripts/build-deb.sh" "$root/scripts/verify-deb.sh" "$root/scripts/validate-staged.sh" "$root/scripts/verify-delivery-assets.sh" "$root/packaging/test-bundle.sh" "$root/packaging/test-deb.sh" "$root/packaging/bundle/install.sh" "$root/packaging/bundle/uninstall.sh" "$root/packaging/bundle/scripts/cnftctl-recover" "$root/packaging/bundle/scripts/inspect-transaction" "$root/packaging/bundle/scripts/verify-bundle" "$root"/packaging/debian/preinst "$root"/packaging/debian/postinst "$root"/packaging/debian/prerm "$root"/packaging/debian/postrm; do
    sh -n "$script"
done

for unit in "$root"/deploy/systemd/*; do
    systemd-analyze verify "$unit" 2>&1 | while IFS= read -r line; do
        case $line in
            *'Command /usr/bin/cnftctl is not executable: No such file or directory'*) ;;
            *'Command /usr/sbin/nft is not executable: No such file or directory'*) ;;
            *'Command /usr/lib/cnftctl/cnftctl-recover is not executable: No such file or directory'*) ;;
            '') ;;
            *) echo "$line" >&2; exit 1 ;;
        esac
    done
done

grep -q '^ExecStart=/usr/sbin/nft -I /var/lib/cnftctl/active -f /var/lib/cnftctl/active/firewall.nft$' "$root/deploy/systemd/cnftctl-firewall.service"
grep -q '^ExecStart=/usr/bin/cnftctl rollback %i$' "$root/deploy/systemd/cnftctl-rollback@.service"
grep -q '^ExecStart=/usr/bin/cnftctl reconcile$' "$root/deploy/systemd/cnftctl-reconcile.service"
grep -q '^RemainAfterExit=yes$' "$root/deploy/systemd/cnftctl-reconcile.service"
grep -q '^RuntimeDirectory=cnftctl$' "$root/deploy/systemd/cnftctl-reconcile.service"
grep -q '^RuntimeDirectory=cnftctl$' "$root/deploy/systemd/cnftctl-rollback@.service"
grep -q '^RuntimeDirectory=cnftctl$' "$root/deploy/systemd/cnftctl-ddns-refresh.service"
if grep -Eq '(systemctl|systemctl_cmd).* (enable|start|enable --now|start --no-block).*cnftctl-firewall' "$root/packaging/bundle/install.sh" "$root/packaging/debian/postinst"; then
    echo "installer must not activate the firewall unit" >&2
    exit 1
fi
grep -q 'name: Release Candidate Build$' "$root/.github/workflows/release-build.yml"
# Match literal GitHub expression syntax.
# shellcheck disable=SC2016
grep -q 'actions/workflows/release-build.yml@\$GITHUB_SHA' "$root/.github/workflows/release-promote.yml"
# Match literal GitHub expression syntax.
# shellcheck disable=SC2016
grep -q 'cnftctl_${{ inputs.version }}_amd64.deb' "$root/.github/workflows/release-build.yml"

sh "$root/packaging/test-bundle.sh"
sh "$root/packaging/test-deb.sh"
