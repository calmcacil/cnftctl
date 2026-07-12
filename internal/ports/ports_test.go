package ports

import (
	"strings"
	"testing"
)

func TestOpenIsIdempotentAndWarnsForDocker(t *testing.T) {
	cfg := &Config{DockerEnabled: true}

	res, err := Open(cfg, "TCP", "443", "https")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed || len(cfg.OpenPorts) != 1 {
		t.Fatalf("expected first open to change config, got changed=%v ports=%d", res.Changed, len(cfg.OpenPorts))
	}
	if len(res.Warnings) != 1 || res.Warnings[0].Code != WarningDockerExposure {
		t.Fatalf("expected docker exposure warning, got %#v", res.Warnings)
	}

	res, err = Open(cfg, "tcp", "443", "ignored")
	if err != nil {
		t.Fatal(err)
	}
	if res.Changed || len(cfg.OpenPorts) != 1 || cfg.OpenPorts[0].Comment != "https" {
		t.Fatalf("duplicate open should be a no-op preserving comment: %#v", cfg.OpenPorts)
	}
}

func TestCloseStrictAndNonStrict(t *testing.T) {
	cfg := &Config{OpenPorts: []Entry{{Protocol: "udp", Start: 41641, End: 41641}}}

	res, err := Close(cfg, "udp", "53", false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Changed || len(cfg.OpenPorts) != 1 {
		t.Fatalf("non-strict missing close should be no-op, got changed=%v ports=%#v", res.Changed, cfg.OpenPorts)
	}

	if _, err := Close(cfg, "udp", "53", true); err == nil {
		t.Fatal("strict missing close should fail")
	}

	res, err = Close(cfg, "udp", "41641", true)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed || len(cfg.OpenPorts) != 0 {
		t.Fatalf("expected strict existing close to remove port, got changed=%v ports=%#v", res.Changed, cfg.OpenPorts)
	}
}

func TestRejectInvalidPorts(t *testing.T) {
	for _, tc := range []struct{ proto, port string }{{"icmp", "443"}, {"tcp", "0"}, {"tcp", "65536"}, {"tcp", "100-10"}} {
		if _, err := Open(&Config{}, tc.proto, tc.port, ""); err == nil {
			t.Fatalf("expected error for %s %s", tc.proto, tc.port)
		}
	}
}

func TestRejectCommentControlCharacters(t *testing.T) {
	if _, err := Open(&Config{}, "tcp", "443", "unsafe\ncomment"); err == nil {
		t.Fatal("expected control character rejection")
	}
}

func FuzzParseEntry(f *testing.F) {
	f.Add("tcp", "443", "https")
	f.Add("udp", "1-65535", "")
	f.Fuzz(func(t *testing.T, protocol, spec, comment string) {
		entry, err := ParseEntry(protocol, spec, comment)
		if err != nil {
			return
		}
		if entry.Start == 0 || entry.End < entry.Start || entry.End == 0 {
			t.Fatalf("invalid accepted entry: %#v", entry)
		}
		if strings.ContainsAny(FormatPort(entry), " \t\r\n") {
			t.Fatalf("unsafe formatted port: %q", FormatPort(entry))
		}
	})
}
