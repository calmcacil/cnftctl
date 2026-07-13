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
# Match the signer identity emitted by attest-build-provenance for a build run
# dispatched from protected main. Candidate run head_sha remains pinned to the
# checked-out release tag commit by the promotion workflow.
# shellcheck disable=SC2016
grep -q 'signer_host/\$GITHUB_REPOSITORY/.github/workflows/release-build.yml@refs/heads/main' "$root/.github/workflows/release-promote.yml"
# shellcheck disable=SC2016
grep -q -- '--signer-digest "$target_sha"' "$root/.github/workflows/release-promote.yml"
# shellcheck disable=SC2016
grep -q -- '--source-digest "$target_sha"' "$root/.github/workflows/release-promote.yml"
grep -q -- '--source-ref refs/heads/main' "$root/.github/workflows/release-promote.yml"
# Match literal GitHub expression syntax.
# shellcheck disable=SC2016
grep -q 'ref: v${{ inputs.version }}' "$root/.github/workflows/release-promote.yml"
# Match literal GitHub expression syntax.
# shellcheck disable=SC2016
grep -q 'cnftctl_${{ inputs.version }}_${{ matrix.arch }}.deb' "$root/.github/workflows/release-build.yml"
grep -q 'ubuntu-24.04-arm' "$root/.github/workflows/ci.yml" "$root/.github/workflows/release-build.yml"
# shellcheck disable=SC2016
grep -q 'cnftctl_${VERSION}_arm64.deb' "$root/.github/workflows/release-promote.yml"

sh "$root/packaging/test-bundle.sh"
sh "$root/packaging/test-deb.sh"
