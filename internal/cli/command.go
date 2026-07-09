package cli

import "fmt"

type command struct {
	Use         string
	Summary     string
	Args        string
	Flags       []flagSpec
	Children    []*command
	HandlerName string
}

type flagSpec struct {
	Name      string
	Short     string
	Usage     string
	ValueName string
	Bool      bool
	Repeat    bool
	Default   string
}

func (c *command) path() []string {
	if c.Use == "" {
		return nil
	}
	return []string{c.Use}
}

func (c *command) findChild(name string) *command {
	for _, child := range c.Children {
		if child.Use == name {
			return child
		}
	}
	return nil
}

func commandTree() *command {
	return &command{
		Use:     "cnftctl",
		Summary: "Manage the cnftctl nftables firewall profile.",
		Flags: []flagSpec{
			{Name: "help", Short: "h", Usage: "show help", Bool: true},
			{Name: "version", Usage: "show version", Bool: true},
			{Name: "config", Usage: "config file path", ValueName: "path", Default: "/etc/cnftctl/config.yaml"},
			{Name: "root", Usage: "alternate filesystem root for previews/tests", ValueName: "path"},
		},
		Children: []*command{
			{Use: "status", Summary: "Show installed profile status.", HandlerName: "status"},
			{Use: "config", Summary: "Inspect cnftctl configuration.", Children: []*command{
				{Use: "show", Summary: "Print the active config.", HandlerName: "config show"},
			}},
			{Use: "init", Summary: "Create initial config and managed files.", HandlerName: "init", Flags: []flagSpec{
				{Name: "dry-run", Usage: "preview without writing", Bool: true},
				{Name: "wan-interface", Usage: "WAN interface name", ValueName: "name"},
				{Name: "enable-docker", Usage: "enable Docker WAN gating", Bool: true},
				{Name: "trust-interface", Usage: "trusted overlay/VPN interface", ValueName: "name", Repeat: true},
				{Name: "enable-ddns-whitelist", Usage: "enable DDNS SSH whitelist", Bool: true},
				{Name: "preset", Usage: "base64url JSON preset", ValueName: "value"},
				{Name: "preset-file", Usage: "preset JSON file", ValueName: "path"},
				{Name: "yes", Short: "y", Usage: "skip interactive confirmation", Bool: true},
			}},
			{Use: "validate", Summary: "Validate config and rendered nftables.", HandlerName: "validate", Flags: []flagSpec{
				{Name: "output-root", Usage: "alternate render root", ValueName: "path"},
			}},
			{Use: "plan", Summary: "Show pending file and policy changes.", HandlerName: "plan", Flags: []flagSpec{
				{Name: "output-root", Usage: "alternate render root", ValueName: "path"},
			}},
			{Use: "apply", Summary: "Apply changes with dead-man rollback.", HandlerName: "apply", Flags: []flagSpec{
				{Name: "dry-run", Usage: "preview without writing or loading", Bool: true},
				{Name: "rollback-timeout", Usage: "dead-man rollback timeout", ValueName: "duration", Default: "120s"},
			}},
			{Use: "confirm", Summary: "Confirm a pending apply transaction.", HandlerName: "confirm"},
			{Use: "rollback", Summary: "Rollback a pending apply transaction.", HandlerName: "rollback", Flags: []flagSpec{
				{Name: "transaction-dir", Usage: "transaction directory to restore", ValueName: "path"},
			}},
			{Use: "open", Summary: "Add a public WAN open port.", Args: "<tcp|udp> <port-or-range>", HandlerName: "open", Flags: []flagSpec{
				{Name: "comment", Usage: "operator comment", ValueName: "text"},
			}},
			{Use: "close", Summary: "Remove a public WAN open port.", Args: "<tcp|udp> <port-or-range>", HandlerName: "close", Flags: []flagSpec{
				{Name: "strict", Usage: "fail if the port is not configured", Bool: true},
			}},
			{Use: "ports", Summary: "Manage public WAN open ports.", Children: []*command{
				{Use: "list", Summary: "List configured public WAN ports.", HandlerName: "ports list"},
			}},
			{Use: "whitelist", Summary: "Manage static SSH allowlists.", Children: []*command{
				{Use: "add", Summary: "Add a static SSH allowlist CIDR/address.", Args: "<ip-or-cidr>", HandlerName: "whitelist add", Flags: []flagSpec{{Name: "comment", Usage: "operator comment", ValueName: "text"}}},
				{Use: "remove", Summary: "Remove a static SSH allowlist CIDR/address.", Args: "<ip-or-cidr>", HandlerName: "whitelist remove", Flags: []flagSpec{{Name: "strict", Usage: "fail if the entry is not configured", Bool: true}}},
				{Use: "list", Summary: "List static SSH allowlist entries.", HandlerName: "whitelist list"},
			}},
			{Use: "ddns", Summary: "Manage DDNS SSH allowlists.", Children: []*command{
				{Use: "enable", Summary: "Enable DDNS SSH allowlisting.", HandlerName: "ddns enable"},
				{Use: "disable", Summary: "Disable DDNS SSH allowlisting.", HandlerName: "ddns disable"},
				{Use: "add", Summary: "Add a DDNS hostname.", Args: "<hostname>", HandlerName: "ddns add"},
				{Use: "remove", Summary: "Remove a DDNS hostname.", Args: "<hostname>", HandlerName: "ddns remove", Flags: []flagSpec{{Name: "strict", Usage: "fail if the host is not configured", Bool: true}}},
				{Use: "refresh", Summary: "Refresh DDNS runtime nftables sets.", HandlerName: "ddns refresh"},
				{Use: "status", Summary: "Show DDNS whitelist status.", HandlerName: "ddns status"},
				{Use: "set-ipv6-prefix-len", Summary: "Set DDNS IPv6 prefix length.", Args: "<56|64>", HandlerName: "ddns set-ipv6-prefix-len"},
			}},
			{Use: "ssh-harden", Summary: "Set SSH exposure mode.", Children: []*command{
				{Use: "open", Summary: "Keep SSH open from WAN.", HandlerName: "ssh-harden open"},
				{Use: "whitelist-only", Summary: "Allow SSH only from allowlists.", HandlerName: "ssh-harden whitelist-only", Flags: []flagSpec{{Name: "force", Usage: "allow potentially unsafe hardening", Bool: true}}},
				{Use: "whitelist-rate-limit", Summary: "Allowlisted SSH with rate limits.", HandlerName: "ssh-harden whitelist-rate-limit", Flags: []flagSpec{{Name: "force", Usage: "allow potentially unsafe hardening", Bool: true}}},
			}},
			{Use: "feature", Summary: "Toggle optional firewall features.", Children: []*command{
				{Use: "enable", Summary: "Enable an optional feature.", Args: "<docker|trusted-interface>", HandlerName: "feature enable", Flags: []flagSpec{{Name: "interface", Short: "i", Usage: "trusted interface name", ValueName: "name", Repeat: true}}},
				{Use: "disable", Summary: "Disable an optional feature.", Args: "<docker|trusted-interface>", HandlerName: "feature disable", Flags: []flagSpec{{Name: "interface", Short: "i", Usage: "trusted interface name", ValueName: "name", Repeat: true}}},
			}},
			{Use: "preset", Summary: "Decode, validate, and explain presets.", Children: []*command{
				{Use: "decode", Summary: "Decode a base64url JSON preset.", Args: "<preset>", HandlerName: "preset decode"},
				{Use: "validate", Summary: "Validate a preset JSON file.", Args: "<file>", HandlerName: "preset validate"},
				{Use: "explain", Summary: "Explain preset impact and risks.", Args: "<file>", HandlerName: "preset explain"},
			}},
		},
	}
}

func usageFor(path []string, c *command) string {
	use := "cnftctl"
	if len(path) > 0 {
		use = join(path, " ")
	}
	if len(c.Children) > 0 {
		return fmt.Sprintf("%s <command>", use)
	}
	if c.Args != "" {
		return fmt.Sprintf("%s %s", use, c.Args)
	}
	return use
}
