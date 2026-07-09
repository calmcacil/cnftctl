package whitelist

import "testing"

func TestAddRejectsHostnamesAndWarnsBroadPrefixes(t *testing.T) {
	cfg := &Config{}
	if _, err := Add(cfg, "home.example.com", ""); err == nil {
		t.Fatal("expected hostname rejection")
	}

	res, err := Add(cfg, "198.51.100.0/24", "office")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed || len(res.Warnings) != 1 || res.Warnings[0].Code != WarningBroadPrefix {
		t.Fatalf("expected broad prefix warning, got %#v", res)
	}
}

func TestAddRemoveAreIdempotent(t *testing.T) {
	cfg := &Config{}
	if res, err := Add(cfg, "203.0.113.10", "home"); err != nil || !res.Changed {
		t.Fatalf("expected add change, result=%#v err=%v", res, err)
	}
	if res, err := Add(cfg, "203.0.113.10", "ignored"); err != nil || res.Changed || len(cfg.Static) != 1 || cfg.Static[0].Comment != "home" {
		t.Fatalf("expected duplicate add no-op preserving comment, result=%#v err=%v config=%#v", res, err, cfg)
	}
	if res, err := Remove(cfg, "203.0.113.11"); err != nil || res.Changed || len(cfg.Static) != 1 {
		t.Fatalf("expected missing remove no-op, result=%#v err=%v config=%#v", res, err, cfg)
	}
	if res, err := Remove(cfg, "203.0.113.10"); err != nil || !res.Changed || len(cfg.Static) != 0 {
		t.Fatalf("expected remove change, result=%#v err=%v config=%#v", res, err, cfg)
	}
}
