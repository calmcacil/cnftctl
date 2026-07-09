package ddns

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sort"
	"strings"
	"time"
)

const DefaultIPv6PrefixLen = 56

type Config struct {
	Enabled       bool
	Hosts         []string
	IPv6PrefixLen int
	TTL           time.Duration
}

type Resolver interface {
	LookupA(ctx context.Context, host string) ([]netip.Addr, error)
	LookupAAAA(ctx context.Context, host string) ([]netip.Addr, error)
}

type NetResolver struct{}

type RuntimeSet interface {
	Refresh(ctx context.Context, ipv4 []netip.Addr, ipv6 []netip.Prefix, ttl time.Duration) error
	List(ctx context.Context) (RuntimeEntries, error)
}

type RuntimeEntries struct {
	IPv4 []netip.Addr
	IPv6 []netip.Prefix
}

type RefreshResult struct {
	IPv4 []netip.Addr
	IPv6 []netip.Prefix
}

type Status struct {
	Enabled       bool
	Hosts         []string
	IPv6PrefixLen int
	Configured    RefreshResult
	Runtime       RuntimeEntries
	RuntimeError  error
}

func Enable(cfg *Config) (bool, error) {
	if cfg == nil {
		return false, errors.New("DDNS config is nil")
	}
	ensureDefaults(cfg)
	changed := !cfg.Enabled
	cfg.Enabled = true
	return changed, nil
}

func Disable(cfg *Config) (bool, error) {
	if cfg == nil {
		return false, errors.New("DDNS config is nil")
	}
	changed := cfg.Enabled
	cfg.Enabled = false
	return changed, nil
}

func AddHost(cfg *Config, host string) (bool, error) {
	if cfg == nil {
		return false, errors.New("DDNS config is nil")
	}
	host, err := normalizeHost(host)
	if err != nil {
		return false, err
	}
	for _, existing := range cfg.Hosts {
		if existing == host {
			return false, nil
		}
	}
	cfg.Hosts = append(cfg.Hosts, host)
	sort.Strings(cfg.Hosts)
	ensureDefaults(cfg)
	return true, nil
}

func RemoveHost(cfg *Config, host string) (bool, error) {
	if cfg == nil {
		return false, errors.New("DDNS config is nil")
	}
	host, err := normalizeHost(host)
	if err != nil {
		return false, err
	}
	for i, existing := range cfg.Hosts {
		if existing == host {
			cfg.Hosts = append(cfg.Hosts[:i], cfg.Hosts[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}

func SetIPv6PrefixLen(cfg *Config, prefixLen int) (bool, error) {
	if cfg == nil {
		return false, errors.New("DDNS config is nil")
	}
	if prefixLen != 56 && prefixLen != 64 {
		return false, fmt.Errorf("unsupported DDNS IPv6 prefix length %d: must be 56 or 64", prefixLen)
	}
	changed := cfg.IPv6PrefixLen != prefixLen
	cfg.IPv6PrefixLen = prefixLen
	return changed, nil
}

func Refresh(ctx context.Context, cfg Config, resolver Resolver, runtime RuntimeSet) (RefreshResult, error) {
	ensureDefaults(&cfg)
	if !cfg.Enabled {
		return RefreshResult{}, errors.New("DDNS whitelist is disabled")
	}
	if len(cfg.Hosts) == 0 {
		return RefreshResult{}, errors.New("no DDNS hosts configured")
	}
	if resolver == nil {
		return RefreshResult{}, errors.New("resolver is nil")
	}
	if runtime == nil {
		return RefreshResult{}, errors.New("runtime set is nil")
	}

	result, err := Resolve(ctx, cfg, resolver)
	if err != nil {
		return RefreshResult{}, err
	}
	if len(result.IPv4) == 0 && len(result.IPv6) == 0 {
		return RefreshResult{}, errors.New("resolved DDNS hosts but found no usable A or AAAA records")
	}

	// Apply only after every configured host resolves successfully. If resolution
	// fails, the existing nftables runtime sets are left untouched.
	if err := runtime.Refresh(ctx, result.IPv4, result.IPv6, cfg.TTL); err != nil {
		return RefreshResult{}, err
	}
	return result, nil
}

func Resolve(ctx context.Context, cfg Config, resolver Resolver) (RefreshResult, error) {
	ensureDefaults(&cfg)
	if resolver == nil {
		return RefreshResult{}, errors.New("resolver is nil")
	}

	ipv4Seen := map[netip.Addr]struct{}{}
	ipv6Seen := map[netip.Prefix]struct{}{}
	var result RefreshResult

	for _, host := range cfg.Hosts {
		host, err := normalizeHost(host)
		if err != nil {
			return RefreshResult{}, err
		}

		addrs4, err := resolver.LookupA(ctx, host)
		if err != nil {
			return RefreshResult{}, fmt.Errorf("resolve A for %s: %w", host, err)
		}
		for _, addr := range addrs4 {
			if !addr.Is4() {
				continue
			}
			addr = addr.Unmap()
			if _, ok := ipv4Seen[addr]; !ok {
				ipv4Seen[addr] = struct{}{}
				result.IPv4 = append(result.IPv4, addr)
			}
		}

		addrs6, err := resolver.LookupAAAA(ctx, host)
		if err != nil {
			return RefreshResult{}, fmt.Errorf("resolve AAAA for %s: %w", host, err)
		}
		for _, addr := range addrs6 {
			prefix, ok := DeriveIPv6Prefix(addr, cfg.IPv6PrefixLen)
			if !ok {
				continue
			}
			if _, seen := ipv6Seen[prefix]; !seen {
				ipv6Seen[prefix] = struct{}{}
				result.IPv6 = append(result.IPv6, prefix)
			}
		}
	}

	sort.Slice(result.IPv4, func(i, j int) bool { return result.IPv4[i].Compare(result.IPv4[j]) < 0 })
	sort.Slice(result.IPv6, func(i, j int) bool {
		if cmp := result.IPv6[i].Addr().Compare(result.IPv6[j].Addr()); cmp != 0 {
			return cmp < 0
		}
		return result.IPv6[i].Bits() < result.IPv6[j].Bits()
	})
	return result, nil
}

func StatusOf(ctx context.Context, cfg Config, resolver Resolver, runtime RuntimeSet) Status {
	ensureDefaults(&cfg)
	status := Status{Enabled: cfg.Enabled, Hosts: append([]string(nil), cfg.Hosts...), IPv6PrefixLen: cfg.IPv6PrefixLen}
	if cfg.Enabled && resolver != nil && len(cfg.Hosts) > 0 {
		status.Configured, _ = Resolve(ctx, cfg, resolver)
	}
	if runtime != nil {
		status.Runtime, status.RuntimeError = runtime.List(ctx)
	}
	return status
}

func DeriveIPv6Prefix(addr netip.Addr, prefixLen int) (netip.Prefix, bool) {
	if prefixLen != 56 && prefixLen != 64 {
		return netip.Prefix{}, false
	}
	addr = addr.Unmap()
	if !addr.Is6() || addr.Is4() {
		return netip.Prefix{}, false
	}
	return netip.PrefixFrom(addr, prefixLen).Masked(), true
}

func (NetResolver) LookupA(ctx context.Context, host string) ([]netip.Addr, error) {
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", host)
	if err != nil {
		return nil, err
	}
	return netIPsToAddrs(ips), nil
}

func (NetResolver) LookupAAAA(ctx context.Context, host string) ([]netip.Addr, error) {
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip6", host)
	if err != nil {
		return nil, err
	}
	return netIPsToAddrs(ips), nil
}

func netIPsToAddrs(ips []net.IP) []netip.Addr {
	addrs := make([]netip.Addr, 0, len(ips))
	for _, ip := range ips {
		addr, ok := netip.AddrFromSlice(ip)
		if ok {
			addrs = append(addrs, addr.Unmap())
		}
	}
	return addrs
}

func ensureDefaults(cfg *Config) {
	if cfg.IPv6PrefixLen == 0 {
		cfg.IPv6PrefixLen = DefaultIPv6PrefixLen
	}
	if cfg.TTL == 0 {
		cfg.TTL = time.Hour
	}
}

func normalizeHost(host string) (string, error) {
	host = strings.TrimSpace(strings.TrimSuffix(strings.ToLower(host), "."))
	if host == "" {
		return "", errors.New("DDNS host is required")
	}
	if strings.ContainsAny(host, " /:") {
		return "", fmt.Errorf("invalid DDNS hostname %q", host)
	}
	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		return "", fmt.Errorf("invalid DDNS hostname %q", host)
	}
	for _, label := range labels {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return "", fmt.Errorf("invalid DDNS hostname %q", host)
		}
		for _, r := range label {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
				return "", fmt.Errorf("invalid DDNS hostname %q", host)
			}
		}
	}
	return host, nil
}
