package ddns

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"testing"
	"time"
)

type fakeResolver struct {
	a        map[string][]netip.Addr
	aaaa     map[string][]netip.Addr
	failA    bool
	failAAAA bool
}

func (r fakeResolver) LookupA(_ context.Context, host string) ([]netip.Addr, error) {
	if r.failA {
		return nil, errors.New("boom")
	}
	return r.a[host], nil
}

func (r fakeResolver) LookupAAAA(_ context.Context, host string) ([]netip.Addr, error) {
	if r.failAAAA {
		return nil, errors.New("boom")
	}
	return r.aaaa[host], nil
}

type fakeRuntime struct {
	called  bool
	ipv4    []netip.Addr
	ipv6    []netip.Prefix
	ttl     time.Duration
	list    RuntimeEntries
	listErr error
}

func (r *fakeRuntime) Refresh(_ context.Context, ipv4 []netip.Addr, ipv6 []netip.Prefix, ttl time.Duration) error {
	r.called = true
	r.ipv4 = append([]netip.Addr(nil), ipv4...)
	r.ipv6 = append([]netip.Prefix(nil), ipv6...)
	r.ttl = ttl
	return nil
}

func (r *fakeRuntime) List(context.Context) (RuntimeEntries, error) { return r.list, r.listErr }

func TestHostAndPrefixManagement(t *testing.T) {
	cfg := &Config{}
	if changed, err := Enable(cfg); err != nil || !changed || !cfg.Enabled || cfg.IPv6PrefixLen != 56 {
		t.Fatalf("enable failed: changed=%v err=%v cfg=%#v", changed, err, cfg)
	}
	if _, err := AddHost(cfg, "notahost"); err == nil {
		t.Fatal("expected invalid hostname rejection")
	}
	if changed, err := AddHost(cfg, "Home.Example.Com."); err != nil || !changed || cfg.Hosts[0] != "home.example.com" {
		t.Fatalf("add host failed: changed=%v err=%v cfg=%#v", changed, err, cfg)
	}
	if changed, err := SetIPv6PrefixLen(cfg, 64); err != nil || !changed || cfg.IPv6PrefixLen != 64 {
		t.Fatalf("set prefix len failed: changed=%v err=%v cfg=%#v", changed, err, cfg)
	}
	if _, err := SetIPv6PrefixLen(cfg, 48); err == nil {
		t.Fatal("expected unsupported prefix length rejection")
	}
}

func TestRefreshUsesExactIPv4AndDerivedIPv6Prefix(t *testing.T) {
	cfg := Config{Enabled: true, Hosts: []string{"home.example.com"}, IPv6PrefixLen: 56, TTL: 2 * time.Hour}
	runtime := &fakeRuntime{}
	resolver := fakeResolver{
		a:    map[string][]netip.Addr{"home.example.com": {netip.MustParseAddr("203.0.113.10"), netip.MustParseAddr("2001:db8::1")}},
		aaaa: map[string][]netip.Addr{"home.example.com": {netip.MustParseAddr("2001:db8:1234:5678::abcd"), netip.MustParseAddr("::ffff:203.0.113.10")}},
	}

	res, err := Refresh(context.Background(), cfg, resolver, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if !runtime.called || runtime.ttl != 2*time.Hour {
		t.Fatalf("expected runtime refresh with TTL, got %#v", runtime)
	}
	if len(res.IPv4) != 1 || res.IPv4[0].String() != "203.0.113.10" {
		t.Fatalf("expected exact IPv4 A record only, got %#v", res.IPv4)
	}
	if len(res.IPv6) != 1 || res.IPv6[0].String() != "2001:db8:1234:5600::/56" {
		t.Fatalf("expected derived /56 IPv6 prefix, got %#v", res.IPv6)
	}
}

func TestRefreshDoesNotTouchRuntimeOnResolutionFailure(t *testing.T) {
	cfg := Config{Enabled: true, Hosts: []string{"home.example.com"}}
	runtime := &fakeRuntime{}
	_, err := Refresh(context.Background(), cfg, fakeResolver{failA: true}, runtime)
	if err == nil {
		t.Fatal("expected refresh failure")
	}
	if runtime.called {
		t.Fatal("runtime refresh must not be called when resolution fails")
	}
}

func TestRemoveHostAndDisableAreIdempotent(t *testing.T) {
	cfg := &Config{Enabled: true, Hosts: []string{"home.example.com"}}
	changed, err := RemoveHost(cfg, "HOME.EXAMPLE.COM.")
	if err != nil || !changed || len(cfg.Hosts) != 0 {
		t.Fatalf("remove host failed: changed=%v err=%v cfg=%#v", changed, err, cfg)
	}
	changed, err = RemoveHost(cfg, "missing.example.com")
	if err != nil || changed {
		t.Fatalf("expected missing host no-op, changed=%v err=%v", changed, err)
	}
	changed, err = Disable(cfg)
	if err != nil || !changed || cfg.Enabled {
		t.Fatalf("disable failed: changed=%v err=%v cfg=%#v", changed, err, cfg)
	}
	changed, err = Disable(cfg)
	if err != nil || changed {
		t.Fatalf("expected duplicate disable no-op, changed=%v err=%v", changed, err)
	}
}

func TestStatusOfReportsConfiguredAndRuntimeEntries(t *testing.T) {
	runtimeErr := errors.New("list failed")
	runtime := &fakeRuntime{
		list: RuntimeEntries{
			IPv4: []netip.Addr{netip.MustParseAddr("203.0.113.20")},
			IPv6: []netip.Prefix{netip.MustParsePrefix("2001:db8:ffff::/56")},
		},
		listErr: runtimeErr,
	}
	cfg := Config{Enabled: true, Hosts: []string{"home.example.com"}, IPv6PrefixLen: 56}
	resolver := fakeResolver{aaaa: map[string][]netip.Addr{"home.example.com": {netip.MustParseAddr("2001:db8:1234:5678::1")}}}

	status := StatusOf(context.Background(), cfg, resolver, runtime)
	if !status.Enabled || status.IPv6PrefixLen != 56 || len(status.Hosts) != 1 || status.Hosts[0] != "home.example.com" {
		t.Fatalf("unexpected status config: %#v", status)
	}
	if len(status.Configured.IPv6) != 1 || status.Configured.IPv6[0].String() != "2001:db8:1234:5600::/56" {
		t.Fatalf("unexpected configured entries: %#v", status.Configured)
	}
	if len(status.Runtime.IPv4) != 1 || len(status.Runtime.IPv6) != 1 || !errors.Is(status.RuntimeError, runtimeErr) {
		t.Fatalf("unexpected runtime status: %#v", status)
	}

	status.Hosts[0] = "mutated.example.com"
	if cfg.Hosts[0] != "home.example.com" {
		t.Fatalf("status hosts should not alias config hosts: %#v", cfg.Hosts)
	}
}

func TestNetIPsToAddrsUnmapsIPv4(t *testing.T) {
	addrs := netIPsToAddrs([]net.IP{net.ParseIP("203.0.113.10"), net.ParseIP("::ffff:203.0.113.11"), nil})
	if len(addrs) != 2 || addrs[0].String() != "203.0.113.10" || addrs[1].String() != "203.0.113.11" {
		t.Fatalf("unexpected addresses: %#v", addrs)
	}
}
