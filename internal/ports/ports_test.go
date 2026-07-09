package ports

import "testing"

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
