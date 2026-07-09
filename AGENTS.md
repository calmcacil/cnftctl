# AGENTS.md

## Repository Shape
- This repo is a sanitized reference package under `reference/`; there is no Go module, app build, or test harness yet.
- Treat `reference/README.md` and the shipped config/scripts as the source of truth for deployment behavior.
- Planned direction: evolve this into a Go package/CLI for managing this specific firewall setup, starting with `open-ports.nft` and whitelist management; an initialize/setup subcommand may make sense for bootstrapping a new machine.

## Key Files
- `reference/nftables.conf` is the main firewall ruleset copied to `/etc/nftables.conf`.
- `reference/nftables.d/open-ports.nft` defines the single public WAN allowlist used for both host services and Docker-published services.
- `reference/nftables.d/whitelist.nft` contains static SSH source allowlists; dynamic DDNS allowlists are runtime nftables sets in `nftables.conf`.
- `reference/ddns-whitelist/update-nft-ddns-whitelist` is the server-side POSIX `sh` updater run by the systemd timer.
- `reference/cloudflare-ddns.sh` is an EdgeOS/EdgeRouter POSIX `sh` Cloudflare DDNS updater, not a server-side script.

## Validation And Deployment Commands
- Validate the installed firewall with `nft -c -f /etc/nftables.conf`; the reference config includes `/etc/nftables.d/*.nft`, so direct local validation from the repo will not match deployment paths unless those files are staged under `/etc`.
- After installing the DDNS whitelist updater, run `systemctl daemon-reload` then `systemctl enable --now nft-ddns-whitelist.timer`.
- Run the whitelist updater manually with `/usr/local/sbin/update-nft-ddns-whitelist` and inspect sets with `nft list set inet hostfw ddns_whitelist_v4` and `nft list set inet hostfw ddns_whitelist_v6`.

## Important Constraints
- Do not replace the targeted table cleanup in `nftables.conf` with `flush ruleset`; Docker manages its own nftables tables.
- Keep shell scripts POSIX `sh` compatible and dependency-light. The server updater intentionally uses `nft getent awk sort systemd`; the EdgeRouter DDNS script intentionally uses `sh curl ip awk logger` and avoids `jq`/Python.
- Do not commit real Cloudflare tokens, real domains, or personal whitelist addresses; examples should stay sanitized.
- Any port added to `open_ports.nft` becomes public from WAN for both host input and Docker forwarding paths.
- The DDNS IPv6 whitelist derives a prefix from AAAA records using `IPV6_PREFIXLEN` in `update-nft-ddns-whitelist`; default is `/56`, change to `/64` only for a single-LAN trust model.
