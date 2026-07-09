# Manual Validation Checklist

Use this checklist before the first `cnftctl` release and before applying the firewall on a real remote host.

## Local Developer Checks

- Run `go test ./...`.
- Run `go vet ./...`.
- Run `test -z "$(gofmt -l .)"`.
- Build the CLI: `go build -o ./bin/cnftctl ./cmd/cnftctl`.
- Confirm CI passes on the release branch or tag.
- Confirm examples contain only documentation values such as `example.com`, `home.example.com`, `203.0.113.0/24`, `198.51.100.0/24`, and `2001:db8::/32`.
- Confirm no generated or example file contains Cloudflare tokens, private domains, private keys, passwords, or personal allowlist addresses.

## Reference Firewall Syntax Check

The reference rules include absolute `/etc/nftables.d/*.nft` paths, so validate them from a staged system path or disposable host rather than directly from the repository checkout.

1. Back up any existing firewall files on the validation host.
2. Stage `reference/nftables.conf` as `/etc/nftables.conf`.
3. Stage `reference/nftables.d/open-ports.nft` as `/etc/nftables.d/open-ports.nft`.
4. Stage `reference/nftables.d/whitelist.nft` as `/etc/nftables.d/whitelist.nft`.
5. Replace placeholder interface names and whitelist entries with validation-host-safe values.
6. Run `sudo nft -c -f /etc/nftables.conf`.
7. Do not run `sudo nft -f /etc/nftables.conf` until rollback and console access are ready.

## Dead-Man Rollback Drill

Do not perform a remote active-policy test without an out-of-band recovery path.

Validate the rollback behavior for `cnftctl apply` once implemented:

- Start from a known-good firewall state.
- Run `sudo cnftctl plan` and review every file/rule change.
- Run `sudo cnftctl apply` from an SSH session.
- Confirm the output states the rollback deadline and the exact `cnftctl confirm` command.
- Let one test transaction expire without confirmation and verify the previous known-good rules/files are restored.
- Run a second test transaction, execute `sudo cnftctl confirm` before the deadline, and verify no rollback occurs.
- Terminate the initiating SSH session during a pending transaction and verify rollback still happens.
- Verify a concurrent second `apply` is rejected while a transaction is pending.

## First Host Smoke Test

- Confirm `cnftctl status` reports config presence, managed file presence, nftables validation state, DDNS timer state when enabled, Docker integration state, and trusted interface state.
- Confirm `cnftctl init --dry-run` writes nothing under `/etc`, `/run`, or `/usr/local`.
- Confirm a fresh config opens no public WAN service ports beyond the selected SSH mode.
- Confirm SSH remains reachable from the intended administrative path before hardening.
- Confirm `cnftctl validate` fails before writing or loading when config is invalid.
- Confirm `cnftctl open tcp 443` changes config/rendered output but does not load active nftables policy until `apply`.
- Confirm `cnftctl close tcp 443` is idempotent for missing entries unless strict behavior is explicitly requested.

## Docker Validation

Only run this section on a host where Docker disruption is acceptable.

- Confirm Docker integration is disabled by default.
- Confirm enabling Docker explains that `open_ports` affects both host services and Docker-published services.
- Confirm Docker-published WAN traffic is blocked unless the matching protocol/port is present in `open_ports`.
- Confirm IPv4 DNATed Docker-published services are gated by original public destination port.
- Confirm IPv6 DNATed or routed Docker container services are gated by destination port.
- Confirm `cnftctl` does not edit `/etc/docker/daemon.json` or restart Docker without explicit consent.
- If testing Docker's nftables backend migration, back up `/etc/docker/daemon.json` and expect running containers to be disrupted by Docker restart.

## DDNS Whitelist Validation

- Confirm DDNS whitelist is disabled by default.
- Confirm hostnames are stored separately from static SSH allowlist entries.
- Confirm static allowlist commands reject hostnames.
- Confirm A records become exact IPv4 entries in `ddns_whitelist_v4`.
- Confirm AAAA records become IPv6 prefixes in `ddns_whitelist_v6`.
- Confirm the default IPv6 prefix length is `/56`.
- Confirm `/64` is accepted for a single-LAN trust model.
- Confirm entries expire if refresh stops.
- If using the reference shell updater, run `sudo /usr/local/sbin/update-nft-ddns-whitelist` and inspect sets with `sudo nft list set inet hostfw ddns_whitelist_v4` and `sudo nft list set inet hostfw ddns_whitelist_v6`.

## Preset Validation

- Confirm unknown preset schema versions are rejected.
- Confirm presets cannot apply policy by themselves.
- Confirm preset import shows a risk explanation before writing files.
- Confirm public open ports, SSH hardening, broad CIDR prefixes, Docker integration, Docker daemon changes, DDNS hosts, and trusted interfaces are called out in the explanation.
- Confirm malformed base64url input fails clearly.
- Confirm presets containing secrets are rejected or explicitly warned against before any persistence.

## Release Gate

- CI passes for the target commit.
- Disabled release workflow has been reviewed before being moved into active workflows.
- `docs/release-notes.md` has current known limitations.
- The release artifact, if produced, is named `cnftctl` and not `nftctl`.
- Checksums are generated and ready to publish with binary artifacts once release publishing is enabled.
- Manual validation results are recorded in the release notes or release issue.
