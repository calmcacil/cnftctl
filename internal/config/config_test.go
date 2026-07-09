package config

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"
)

func TestDefaultValidates(t *testing.T) {
	cfg := Default()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config should validate: %v", err)
	}
	if cfg.Version != CurrentVersion || cfg.SSH.Mode != "open" || cfg.SSH.DDNSWhitelist.IPv6PrefixLen != 56 {
		t.Fatalf("unexpected defaults: %#v", cfg)
	}
	if len(cfg.Docker.Interfaces) != 2 || cfg.Docker.Interfaces[1] != "br-*" {
		t.Fatalf("unexpected docker defaults: %#v", cfg.Docker.Interfaces)
	}
}

func TestLoadFixtureAndStableSave(t *testing.T) {
	f, err := os.Open("testdata/valid.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cfg, err := Load(f)
	if err != nil {
		t.Fatalf("load valid fixture: %v", err)
	}
	if cfg.SSH.RateLimit == nil || cfg.SSH.RateLimit.Per.Duration != time.Minute {
		t.Fatalf("rate limit not parsed: %#v", cfg.SSH.RateLimit)
	}
	var first bytes.Buffer
	if err := Save(&first, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	var second bytes.Buffer
	loaded, err := Load(bytes.NewReader(first.Bytes()))
	if err != nil {
		t.Fatalf("reload saved config: %v", err)
	}
	if err := Save(&second, loaded); err != nil {
		t.Fatalf("save config again: %v", err)
	}
	if first.String() != second.String() {
		t.Fatalf("save output is not stable\nfirst:\n%s\nsecond:\n%s", first.String(), second.String())
	}
	if !strings.HasPrefix(first.String(), "version: 1\nwan_interface: eth0\nopen_ports:") {
		t.Fatalf("unexpected YAML field ordering:\n%s", first.String())
	}
}

func TestLoadRejectsUnknownYAMLFields(t *testing.T) {
	_, err := Load(strings.NewReader("version: 1\nunknown: true\n"))
	if err == nil {
		t.Fatal("expected unknown field error")
	}
}

func TestValidateRejectsUnknownVersion(t *testing.T) {
	cfg := Default()
	cfg.Version = 2
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "unsupported config version 2") {
		t.Fatalf("expected unsupported version error, got %v", err)
	}
}

func TestValidateRejectsBadValues(t *testing.T) {
	cfg := Default()
	cfg.WANInterface = "bad iface"
	cfg.OpenPorts = []OpenPort{{Protocol: "icmp", Port: 0, EndPort: 70000}}
	cfg.SSH.Mode = "closed"
	cfg.SSH.RateLimit = &RateLimit{Connections: 0, Per: Duration{}}
	cfg.SSH.StaticWhitelist.IPv4 = []string{"example.com", "2001:db8::1"}
	cfg.SSH.StaticWhitelist.IPv6 = []string{"198.51.100.1"}
	cfg.SSH.DDNSWhitelist.Hosts = []string{"-bad.example", "192.0.2.1"}
	cfg.SSH.DDNSWhitelist.TTL = Duration{}
	cfg.SSH.DDNSWhitelist.RefreshInterval = Duration{}
	cfg.SSH.DDNSWhitelist.IPv6PrefixLen = 48
	cfg.TrustedInterfaces.Interfaces = []string{"wg*"}
	cfg.Docker.Interfaces = []string{"*br"}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation errors")
	}
	for _, want := range []string{
		"wan_interface",
		"open_ports[0].protocol",
		"open_ports[0].port",
		"open_ports[0].end_port",
		"ssh.mode",
		"ssh.rate_limit.connections",
		"ssh.static_whitelist.ipv4[0]",
		"ssh.static_whitelist.ipv4[1]",
		"ssh.static_whitelist.ipv6[0]",
		"ssh.ddns_whitelist.hosts[0]",
		"ssh.ddns_whitelist.hosts[1]",
		"ssh.ddns_whitelist.ttl",
		"ssh.ddns_whitelist.refresh_interval",
		"ssh.ddns_whitelist.ipv6_prefix_len",
		"trusted_interfaces.interfaces[0]",
		"docker.interfaces[0]",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected error for %s, got %v", want, err)
		}
	}
}

func TestRiskExplanations(t *testing.T) {
	cfg := Default()
	cfg.OpenPorts = []OpenPort{{Protocol: "tcp", Port: 443}}
	cfg.SSH.Mode = "whitelist-only"
	cfg.SSH.StaticWhitelist.IPv4 = []string{"198.51.100.0/24"}
	cfg.SSH.DDNSWhitelist.Enabled = true
	cfg.TrustedInterfaces.Enabled = true
	cfg.TrustedInterfaces.TrustForwarding = true
	cfg.Docker.Enabled = true
	cfg.Docker.AllowPublishedPortsByDefault = true
	risks := strings.Join(cfg.RiskExplanations(), "\n")
	for _, want := range []string{"public tcp port 443", "whitelist-only", "broad SSH static whitelist", "DDNS", "trusted interfaces", "forwarding", "Docker integration", "allow_published_ports_by_default"} {
		if !strings.Contains(risks, want) {
			t.Fatalf("expected risk %q in:\n%s", want, risks)
		}
	}
}
