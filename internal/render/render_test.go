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
