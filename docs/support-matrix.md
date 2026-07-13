# Support Matrix

The table defines the sole intended production target. Support for a version is established only after its canonical Debian 13 amd64 package has complete exact-artifact evidence.

Current blockers and the mandatory evidence gate are tracked in `docs/production-readiness.md`; candidate results belong in `docs/validation-record.md`.

| Component | Supported | Notes |
| --- | --- | --- |
| Distribution | Debian 13 (`trixie`) | Exact production target. |
| Architecture | `amd64` | Package metadata, installation guards, and runtime status enforce/report this target. |
| Init/service manager | systemd | Required for activation, rollback, boot reconciliation, and DDNS scheduling. |
| Firewall | Debian 13 nftables | `inet hostfw` only. |
| Privilege | Root | Required for install and live policy operations. |
| Docker | Optional | Strict gate; Docker nftables backend migration remains operator-controlled. |
| IPv4/IPv6 | Both | DDNS IPv6 derivation supports `/56` and `/64`. |
| Remote operation | Supported with recovery | Console/rescue access and mandatory dead-man rollback are operational requirements. |

Explicitly unsupported or untested:

- Debian 12 or earlier, Debian testing beyond 13, Ubuntu, Fedora, RHEL, Alpine, and derivatives.
- `arm64`, `386`, and architectures other than `amd64`.
- OpenRC, runit, containers without systemd, and non-systemd hosts.
- Arbitrary nftables profiles, multiple managed tables, or coexistence with another owner of `inet hostfw`.
- Docker backends or network modes not validated in the exact release evidence.
- Kubernetes/CNI policy, router/firewall appliances, and systems where cnftctl acts as a complete forwarding firewall.
- Binary-only installation without the matching Debian package units and integrity manifest.

An unsupported result from `status` or `doctor` is intentional and exits `1`. It does not imply that the CLI can safely apply policy on that platform.
