package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/calmcacil/cnftctl/internal/config"
	"github.com/calmcacil/cnftctl/internal/render"
)

func TestServiceInitDryRunWritesNothing(t *testing.T) {
	root := t.TempDir()
	var stdout bytes.Buffer
	svc := NewService()
	err := svc.Run(context.Background(), IO{Stdout: &stdout, Stderr: &bytes.Buffer{}}, CommandRequest{
		Command: "init",
		Flags: map[string][]string{
			"root":          {root},
			"dry-run":       {"true"},
			"wan-interface": {"eth0"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "etc/cnftctl/config.yaml")); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote config: %v", err)
	}
	if !strings.Contains(stdout.String(), render.NftablesConfPath) {
		t.Fatalf("dry-run output missing rendered files: %s", stdout.String())
	}
}

func TestServiceOpenUpdatesConfigAndRenderedFile(t *testing.T) {
	root := t.TempDir()
	writeTestConfig(t, root, config.Default())
	var stdout, stderr bytes.Buffer
	svc := NewService()
	err := svc.Run(context.Background(), IO{Stdout: &stdout, Stderr: &stderr}, CommandRequest{
		Command: "open",
		Args:    []string{"tcp", "443"},
		Flags: map[string][]string{
			"root":    {root},
			"comment": {"HTTPS"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadFile(filepath.Join(root, "etc/cnftctl/config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.OpenPorts) != 1 || cfg.OpenPorts[0].Protocol != "tcp" || cfg.OpenPorts[0].Port != 443 {
		t.Fatalf("unexpected open ports: %#v", cfg.OpenPorts)
	}
	data, err := os.ReadFile(filepath.Join(root, strings.TrimPrefix(render.OpenPortsPath, "/")))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "tcp . 443") {
		t.Fatalf("rendered open ports missing tcp 443: %s", string(data))
	}
	if !strings.Contains(stdout.String(), "run cnftctl apply") {
		t.Fatalf("missing apply guidance: %s", stdout.String())
	}
}

func TestServiceCloseStrictRejectsMissingPort(t *testing.T) {
	root := t.TempDir()
	writeTestConfig(t, root, config.Default())
	var stdout, stderr bytes.Buffer
	svc := NewService()
	err := svc.Run(context.Background(), IO{Stdout: &stdout, Stderr: &stderr}, CommandRequest{
		Command: "close",
		Args:    []string{"tcp", "443"},
		Flags: map[string][]string{
			"root":   {root},
			"strict": {"true"},
		},
	})
	if err == nil {
		t.Fatal("expected strict close to reject a missing port")
	}
	if !strings.Contains(err.Error(), "is not open") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestServiceWhitelistAddRemoveAndList(t *testing.T) {
	root := t.TempDir()
	writeTestConfig(t, root, config.Default())
	svc := NewService()

	var stdout, stderr bytes.Buffer
	err := svc.Run(context.Background(), IO{Stdout: &stdout, Stderr: &stderr}, CommandRequest{
		Command: "whitelist add",
		Args:    []string{"203.0.113.10"},
		Flags:   map[string][]string{"root": {root}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "whitelist add 203.0.113.10/32 changed=true") {
		t.Fatalf("unexpected add output: %s", stdout.String())
	}

	stdout.Reset()
	err = svc.Run(context.Background(), IO{Stdout: &stdout, Stderr: &stderr}, CommandRequest{Command: "whitelist list", Flags: map[string][]string{"root": {root}}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(stdout.String()) != "203.0.113.10/32" {
		t.Fatalf("unexpected whitelist list: %q", stdout.String())
	}

	stdout.Reset()
	err = svc.Run(context.Background(), IO{Stdout: &stdout, Stderr: &stderr}, CommandRequest{
		Command: "whitelist remove",
		Args:    []string{"203.0.113.10"},
		Flags:   map[string][]string{"root": {root}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "changed=true") {
		t.Fatalf("unexpected remove output: %s", stdout.String())
	}
	cfg, err := config.LoadFile(filepath.Join(root, "etc/cnftctl/config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.SSH.StaticWhitelist.IPv4) != 0 || len(cfg.SSH.StaticWhitelist.IPv6) != 0 {
		t.Fatalf("expected empty whitelist after remove: %#v", cfg.SSH.StaticWhitelist)
	}
}

func TestServiceDDNSCommandsUpdateConfig(t *testing.T) {
	root := t.TempDir()
	writeTestConfig(t, root, config.Default())
	svc := NewService()
	io := IO{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}

	commands := []CommandRequest{
		{Command: "ddns enable", Flags: map[string][]string{"root": {root}}},
		{Command: "ddns add", Args: []string{"Home.Example.Com."}, Flags: map[string][]string{"root": {root}}},
		{Command: "ddns set-ipv6-prefix-len", Args: []string{"64"}, Flags: map[string][]string{"root": {root}}},
	}
	for _, req := range commands {
		if err := svc.Run(context.Background(), io, req); err != nil {
			t.Fatalf("%s failed: %v", req.Command, err)
		}
	}

	cfg, err := config.LoadFile(filepath.Join(root, "etc/cnftctl/config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.SSH.DDNSWhitelist.Enabled || cfg.SSH.DDNSWhitelist.IPv6PrefixLen != 64 {
		t.Fatalf("unexpected DDNS config: %#v", cfg.SSH.DDNSWhitelist)
	}
	if len(cfg.SSH.DDNSWhitelist.Hosts) != 1 || cfg.SSH.DDNSWhitelist.Hosts[0] != "home.example.com" {
		t.Fatalf("unexpected DDNS hosts: %#v", cfg.SSH.DDNSWhitelist.Hosts)
	}

	var stdout bytes.Buffer
	if err := svc.Run(context.Background(), IO{Stdout: &stdout, Stderr: &bytes.Buffer{}}, CommandRequest{Command: "ddns status", Flags: map[string][]string{"root": {root}}}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"enabled: true", "ipv6_prefix_len: 64", "host: home.example.com"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("status output missing %q: %s", want, stdout.String())
		}
	}
}

func TestServiceFeatureCommands(t *testing.T) {
	root := t.TempDir()
	writeTestConfig(t, root, config.Default())
	svc := NewService()
	var stdout, stderr bytes.Buffer

	err := svc.Run(context.Background(), IO{Stdout: &stdout, Stderr: &stderr}, CommandRequest{
		Command: "feature enable",
		Args:    []string{"docker"},
		Flags:   map[string][]string{"root": {root}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "Docker-published ports remain blocked") {
		t.Fatalf("missing docker warning: %s", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	err = svc.Run(context.Background(), IO{Stdout: &stdout, Stderr: &stderr}, CommandRequest{
		Command: "feature enable",
		Args:    []string{"trusted-interface"},
		Flags: map[string][]string{
			"root":      {root},
			"interface": {"tailscale0"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "fully trusted") {
		t.Fatalf("missing trusted interface warning: %s", stderr.String())
	}

	cfg, err := config.LoadFile(filepath.Join(root, "etc/cnftctl/config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Docker.Enabled || !cfg.TrustedInterfaces.Enabled || len(cfg.TrustedInterfaces.Interfaces) != 1 || cfg.TrustedInterfaces.Interfaces[0] != "tailscale0" {
		t.Fatalf("unexpected feature config: %#v", cfg)
	}
}

func TestServicePresetExplain(t *testing.T) {
	root := t.TempDir()
	cfg := config.Default()
	cfg.OpenPorts = append(cfg.OpenPorts, config.OpenPort{Protocol: "tcp", Port: 443})
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	presetPath := filepath.Join(root, "preset.json")
	if err := os.WriteFile(presetPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	svc := NewService()
	err = svc.Run(context.Background(), IO{Stdout: &stdout, Stderr: &bytes.Buffer{}}, CommandRequest{Command: "preset explain", Args: []string{presetPath}, Flags: map[string][]string{}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "risk warnings:") {
		t.Fatalf("expected risk warnings: %s", stdout.String())
	}
}

func writeTestConfig(t *testing.T, root string, cfg config.Config) {
	t.Helper()
	path := filepath.Join(root, "etc/cnftctl/config.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveFile(path, cfg, 0o600); err != nil {
		t.Fatal(err)
	}
}
