package install

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/calmcacil/cnftctl/internal/apply"
)

func TestCheckRoot(t *testing.T) {
	if err := CheckRoot(func() int { return 1000 }); err == nil {
		t.Fatal("expected non-root error")
	}
	if err := CheckRoot(func() int { return 0 }); err != nil {
		t.Fatal(err)
	}
}

func TestPlanUsesAlternateRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "etc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "etc/nftables.conf"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan, err := Plan(Options{Root: root, Files: []apply.File{{Path: NftablesConfPath, Data: []byte("new")}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Changes) != 1 || plan.Changes[0].Action != "update" {
		t.Fatalf("unexpected plan: %#v", plan)
	}
}

func TestParseOpenPortsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "open-ports.nft")
	data := []byte("set open_ports {\n elements = {\n tcp . 443, # https\n udp . 41641,\n }\n}\n")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	ports, warnings, err := ParseOpenPortsFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v", warnings)
	}
	want := []Port{{Protocol: "tcp", Port: "443"}, {Protocol: "udp", Port: "41641"}}
	if !reflect.DeepEqual(ports, want) {
		t.Fatalf("ports = %#v, want %#v", ports, want)
	}
}

func TestParseWhitelistFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "whitelist.nft")
	data := []byte("define whitelist_v4 = {\n 203.0.113.10,\n 198.51.100.0/24\n}\ndefine whitelist_v6 = { 2001:db8::10 }\n")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	w, warnings, err := ParseWhitelistFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v", warnings)
	}
	if !reflect.DeepEqual(w.IPv4, []string{"203.0.113.10", "198.51.100.0/24"}) {
		t.Fatalf("ipv4 = %#v", w.IPv4)
	}
	if !reflect.DeepEqual(w.IPv6, []string{"2001:db8::10"}) {
		t.Fatalf("ipv6 = %#v", w.IPv6)
	}
}

func TestAdoptReference(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "etc/nftables.d"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "etc/nftables.d/open-ports.nft"), []byte("tcp . 443,\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "etc/nftables.d/whitelist.nft"), []byte("define whitelist_v4 = { 203.0.113.10 }\ndefine whitelist_v6 = { 2001:db8::10 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	adoption, err := AdoptReference(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(adoption.OpenPorts) != 1 || adoption.OpenPorts[0].Port != "443" {
		t.Fatalf("adoption = %#v", adoption)
	}
}

func FuzzAdoptionParsers(f *testing.F) {
	f.Add([]byte("tcp . 443,\n"), []byte("define whitelist_v4 = { 203.0.113.1 }\n"))
	f.Fuzz(func(t *testing.T, portsData, whitelistData []byte) {
		dir := t.TempDir()
		portsPath := filepath.Join(dir, "ports.nft")
		whitelistPath := filepath.Join(dir, "whitelist.nft")
		if err := os.WriteFile(portsPath, portsData, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(whitelistPath, whitelistData, 0o600); err != nil {
			t.Fatal(err)
		}
		_, _, _ = ParseOpenPortsFile(portsPath)
		_, _, _ = ParseWhitelistFile(whitelistPath)
	})
}
