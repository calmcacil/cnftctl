package features

import "testing"

func TestSSHHardeningRequiresCoverageUnlessOverride(t *testing.T) {
	cfg := &Config{}
	if _, err := SetSSHMode(cfg, SSHModeWhitelistOnly, false); err == nil {
		t.Fatal("expected hardening without coverage to fail")
	}

	res, err := SetSSHMode(cfg, SSHModeWhitelistOnly, true)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed || len(res.Warnings) != 1 || res.Warnings[0].Code != WarningSSHHardened {
		t.Fatalf("expected hardening warning, got %#v", res)
	}
}

func TestTrustedInterfaceValidationAndIdempotency(t *testing.T) {
	cfg := &Config{}
	if _, err := EnableTrustedInterface(cfg, "bad iface"); err == nil {
		t.Fatal("expected invalid interface rejection")
	}

	res, err := EnableTrustedInterface(cfg, "tailscale0")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed || !cfg.TrustedInterfaces.Enabled || len(res.Warnings) != 1 || res.Warnings[0].Code != WarningTrustedFullTrust {
		t.Fatalf("expected full-trust warning and enabled feature, got result=%#v config=%#v", res, cfg)
	}

	res, err = EnableTrustedInterface(cfg, "tailscale0")
	if err != nil || res.Changed || len(cfg.TrustedInterfaces.Interfaces) != 1 {
		t.Fatalf("expected duplicate enable no-op, result=%#v err=%v config=%#v", res, err, cfg)
	}

	res, err = DisableTrustedInterface(cfg, "tailscale0")
	if err != nil || !res.Changed || cfg.TrustedInterfaces.Enabled {
		t.Fatalf("expected disable to remove last trusted interface, result=%#v err=%v config=%#v", res, err, cfg)
	}
}

func TestDockerFeatureToggleWarningsAndIdempotency(t *testing.T) {
	cfg := &Config{}
	res, err := EnableDocker(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed || !cfg.Docker.Enabled || len(res.Warnings) != 1 || res.Warnings[0].Code != WarningDockerPublishedWAN {
		t.Fatalf("expected docker enable warning, result=%#v config=%#v", res, cfg)
	}

	res, err = EnableDocker(cfg)
	if err != nil || res.Changed || !cfg.Docker.Enabled || len(res.Warnings) != 1 {
		t.Fatalf("expected duplicate enable warning without change, result=%#v err=%v config=%#v", res, err, cfg)
	}

	res, err = DisableDocker(cfg)
	if err != nil || !res.Changed || cfg.Docker.Enabled || len(res.Warnings) != 0 {
		t.Fatalf("expected docker disable, result=%#v err=%v config=%#v", res, err, cfg)
	}

	res, err = DisableDocker(cfg)
	if err != nil || res.Changed || cfg.Docker.Enabled {
		t.Fatalf("expected duplicate disable no-op, result=%#v err=%v config=%#v", res, err, cfg)
	}
}

func TestSSHHardeningAllowsConfiguredCoverage(t *testing.T) {
	cfg := &Config{SSH: SSHConfig{DDNSEnabled: true, DDNSHosts: []string{"home.example.com"}}}
	res, err := SetSSHMode(cfg, SSHModeWhitelistRateLimit, false)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed || cfg.SSH.Mode != SSHModeWhitelistRateLimit || cfg.SSH.HardeningOverride {
		t.Fatalf("unexpected SSH mode result=%#v config=%#v", res, cfg)
	}
}
