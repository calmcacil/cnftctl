# How To Install cnftctl

> **Trust and recovery warning:** cnftctl was made primarily for personal use.
> Best efforts have been made to test it, but you must decide whether you trust
> its source, release evidence, and safeguards. A firewall mistake can lock you
> out or expose services. Arrange tested console or rescue access before the
> first policy activation.

The only supported delivery format is the Debian package from a GitHub
release. Do not install a standalone binary or the internal staging bundle:
the package supplies the systemd rollback units, recovery helpers, integrity
manifest, and checksums that live activation requires.

## 1. Check The Host

The production-supported platform is Debian 13 (`trixie`) on `amd64`.
Debian 13 `arm64` packages are available experimentally, have not completed
the live-host validation gate, and are used at your own risk.

```sh
cat /etc/os-release
dpkg --print-architecture
systemctl --version
nft --version
```

Stop if the distribution is not Debian 13 or the architecture is neither
`amd64` nor `arm64`. The package pre-install guard also enforces the matching
distribution and architecture.

## 2. Download One Release

The examples below use `v0.2.0`. Replace both occurrences together when a
newer release is published. Download the package, its architecture-matched
SBOM, and the checksum file from the same release.

For production-supported amd64:

```sh
version=0.2.0
gh release download "v$version" --repo calmcacil/cnftctl \
  --pattern "cnftctl_${version}_amd64.deb" \
  --pattern "sbom_amd64.spdx.json" \
  --pattern "release-checksums.txt"
```

For experimental arm64:

```sh
version=0.2.0
gh release download "v$version" --repo calmcacil/cnftctl \
  --pattern "cnftctl_${version}_arm64.deb" \
  --pattern "sbom_arm64.spdx.json" \
  --pattern "release-checksums.txt"
```

The release page is
<https://github.com/calmcacil/cnftctl/releases/latest>. Do not mix files from
different releases or architectures.

## 3. Verify Before Installing

When only one package and one SBOM were downloaded, `--ignore-missing` checks
the files present against the release's four-entry checksum inventory:

```sh
sha256sum --ignore-missing --check release-checksums.txt
```

The command must report `OK` for your package and matching SBOM. If all four
artifacts were downloaded, use the stricter complete check:

```sh
sha256sum --check release-checksums.txt
```

Verify the package's GitHub build attestation as a separate identity check:

```sh
arch=$(dpkg --print-architecture)
gh attestation verify "cnftctl_${version}_${arch}.deb" \
  --repo calmcacil/cnftctl
```

Review the current release notes, validation record, SBOM, and support matrix
before deciding to trust the package:

- [Release notes](release-notes.md)
- [Exact-artifact validation](validation-record-0.2.0.md)
- [Support matrix](support-matrix.md)
- [Production-readiness policy](production-readiness.md)

## 4. Install The Package

```sh
arch=$(dpkg --print-architecture)
sudo apt install "./cnftctl_${version}_${arch}.deb"
cnftctl --version
sudo cnftctl status
```

An arm64 installation prints an additional experimental-risk warning.
Installation is intentionally inert: it does not create `inet hostfw`, load a
firewall policy, enable DDNS, or restart Docker. It installs the CLI, recovery
helpers, systemd units, documentation, and delivery integrity data, and enables
only boot reconciliation.

`status` may exit `1` before initial configuration because absent desired and
active state is reportable but not healthy. This is not an installation
failure; see [Command Reference](commands.md#reporting-output-and-exit-codes).

## 5. Create And Activate The First Policy

Keep console/rescue access and a second administration session available.
Replace `eth0` with the actual WAN interface:

```sh
sudo cnftctl init --wan-interface eth0 --yes
sudo cnftctl config show
sudo cnftctl validate
sudo cnftctl plan
sudo cnftctl apply
```

`init` writes only desired operator intent. `apply` validates and activates an
immutable generation and prints a transaction ID with a fixed 120-second
rollback deadline. Test existing SSH, open a second connection, and inspect
the firewall before confirming:

```sh
sudo cnftctl transactions list --detail
sudo nft list table inet hostfw
sudo cnftctl confirm TRANSACTION_ID
sudo cnftctl status
```

If access is wrong, run `sudo cnftctl rollback TRANSACTION_ID` from the safe
session or allow the timer to restore the previous generation. On a first
activation, timeout removes only `inet hostfw`.

Read [How To Use cnftctl](commands.md) before adding exposure, hardening SSH,
enabling DDNS, trusting an interface, or enabling Docker integration.

## Upgrades

Download and verify the new package exactly as above, read its release notes,
then install it with `apt`:

```sh
sudo apt install "./cnftctl_NEW_VERSION_${arch}.deb"
cnftctl --version
sudo cnftctl doctor
```

The package refuses an upgrade when transaction history is unresolved or
unsafe. A valid upgrade preserves desired configuration, immutable
generations, active selection, ownership, and terminal audit history.
