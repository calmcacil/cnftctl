package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFreshConfigSnapshot(t *testing.T) {
	cfg := Config{WANInterface: "eth0"}
	assertSnapshots(t, cfg, "fresh")

	nft, err := NftablesConf(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(nft, "flush ruleset") {
		t.Fatal("renderer must not emit flush ruleset")
	}
	if strings.Contains(nft, "docker_wan_allow") {
		t.Fatal("docker chains must be omitted when Docker integration is disabled")
	}
	if strings.Contains(nft, "TRUSTED_IFS") {
		t.Fatal("trusted interface rules must be omitted when disabled")
	}
	if !strings.Contains(nft, "iifname $WAN_IF tcp dport 22 accept") {
		t.Fatal("fresh config should keep SSH open from WAN")
	}
}

func TestGenerationFilesUseRelativeIncludesAndNoSystemdUnits(t *testing.T) {
	dir := "/var/lib/cnftctl/generations/0123456789abcdef"
	files, err := GenerationFiles(Config{SSH: SSHConfig{DDNSWhitelist: DDNSWhitelist{Enabled: true}}}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 4 {
		t.Fatalf("files = %#v", files)
	}
	if !strings.Contains(files[0].Content, `include "whitelist.nft"`) || !strings.Contains(files[0].Content, `include "open-ports.nft"`) || !strings.Contains(files[0].Content, OwnershipMarker) {
		t.Fatalf("generation policy lacks relative includes/marker:\n%s", files[0].Content)
	}
	for _, file := range files {
		if strings.Contains(file.Path, "/systemd/") || file.Path == "/etc/nftables.conf" {
			t.Fatalf("generation contains forbidden path %q", file.Path)
		}
	}
}

func TestFilesUseStableGenerationPlaceholder(t *testing.T) {
	files, err := Files(Config{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(files[0].Content, `include "whitelist.nft"`) {
		t.Fatalf("logical firewall does not use relative includes:\n%s", files[0].Content)
	}
}

func TestEmptyOpenPortsOmitsEmptyElementsBlock(t *testing.T) {
	content, err := OpenPorts(Config{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(content, "elements") {
		t.Fatalf("empty open_ports set contains an invalid elements block:\n%s", content)
	}
}

func TestHardenedIPv4OnlyWhitelistOmitsEmptyIPv6SetAndRule(t *testing.T) {
	cfg := Config{SSH: SSHConfig{Mode: SSHWhitelistOnly, StaticWhitelist: StaticWhitelist{IPv4: []string{"203.0.113.10/32"}}}}
	files, err := Files(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(files[0].Content, "$whitelist_v6") || strings.Contains(files[2].Content, "whitelist_v6") {
		t.Fatalf("IPv4-only policy rendered an empty IPv6 whitelist:\n%s\n%s", files[0].Content, files[2].Content)
	}
	if !strings.Contains(files[0].Content, "$whitelist_v4") || !strings.Contains(files[2].Content, "define whitelist_v4") {
		t.Fatalf("IPv4 whitelist missing from hardened policy:\n%s\n%s", files[0].Content, files[2].Content)
	}
}

func TestHardenedDockerDDNSSnapshot(t *testing.T) {
	cfg := Config{
		WANInterface: "ens18",
		OpenPorts: []OpenPort{
			{Protocol: "udp", Port: "41641", Comment: "Tailscale direct connectivity"},
			{Protocol: "tcp", Port: "443", Comment: "HTTPS"},
		},
		SSH: SSHConfig{
			Mode:      SSHWhitelistRateLimit,
			RateLimit: "4/minute burst 2 packets",
			StaticWhitelist: StaticWhitelist{
				IPv4: []string{"203.0.113.10", "198.51.100.0/24"},
				IPv6: []string{"2001:db8::10"},
			},
			DDNSWhitelist: DDNSWhitelist{
				Enabled:         true,
				Hosts:           []string{"home.example.com", "backup.example.com"},
				TTL:             time.Hour,
				RefreshInterval: 5 * time.Minute,
				IPv6PrefixLen:   56,
			},
		},
		TrustedInterfaces: TrustedInterfacesConfig{
			Enabled:         true,
			Interfaces:      []string{"tailscale0", "wg0"},
			TrustForwarding: true,
		},
		Docker: DockerConfig{
			Enabled:    true,
			Interfaces: []string{"docker0", "br-*"},
		},
	}

	assertSnapshots(t, cfg, "hardened")

	nft, err := NftablesConf(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ordered := []string{
		"ip protocol icmp accept",
		"meta l4proto icmpv6 accept",
		"ct state established,related accept",
		"meta l4proto tcp ct state invalid drop",
		"iifname \"lo\" accept",
	}
	last := -1
	for _, needle := range ordered {
		pos := strings.Index(nft, needle)
		if pos == -1 {
			t.Fatalf("missing rule %q", needle)
		}
		if pos < last {
			t.Fatalf("rule %q rendered out of order", needle)
		}
		last = pos
	}
}

func assertSnapshots(t *testing.T, cfg Config, prefix string) {
	t.Helper()
	files, err := Files(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasPrefix(file.Path, "/var/lib/cnftctl/") {
			continue
		}
		name := prefix + snapshotName(file.Path)
		snapshotPath := filepath.Join("testdata", name)
		if os.Getenv("UPDATE_SNAPSHOTS") == "1" {
			if err := os.WriteFile(snapshotPath, []byte(file.Content), 0644); err != nil {
				t.Fatalf("write snapshot %s: %v", name, err)
			}
		}
		want, err := os.ReadFile(snapshotPath)
		if err != nil {
			t.Fatalf("read snapshot %s: %v", name, err)
		}
		if file.Content != string(want) {
			t.Fatalf("snapshot mismatch for %s\n--- want\n%s\n--- got\n%s", name, string(want), file.Content)
		}
	}
}

func snapshotName(path string) string {
	path = strings.TrimPrefix(path, "/")
	path = strings.ReplaceAll(path, "/", "_")
	return "_" + path
}
