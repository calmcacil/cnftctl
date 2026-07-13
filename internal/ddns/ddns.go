package ddns

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	DefaultIPv6PrefixLen = 56
	MetadataPath         = "/var/lib/cnftctl/ddns-runtime.json"
)

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
	Enabled         bool
	Hosts           []string
	IPv6PrefixLen   int
	Configured      RefreshResult
	Runtime         RuntimeEntries
	RuntimeError    error
	ResolutionError error
	Metadata        Metadata
	MetadataError   error
	Stale           bool
}

type Metadata struct {
	Attempts     uint64    `json:"attempts"`
	LastAttempt  time.Time `json:"last_attempt"`
	LastSuccess  time.Time `json:"last_success,omitempty"`
	ErrorCode    string    `json:"error_code,omitempty"`
	ErrorSummary string    `json:"error_summary,omitempty"`
	IPv4Count    int       `json:"ipv4_count"`
	IPv6Count    int       `json:"ipv6_count"`
	ContentHash  string    `json:"content_hash,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
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

		addrs4, err4 := resolver.LookupA(ctx, host)
		if err4 != nil && !isAuthoritativeNoData(err4) {
			return RefreshResult{}, fmt.Errorf("resolve A for %s: %w", host, err4)
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

		addrs6, err6 := resolver.LookupAAAA(ctx, host)
		if err6 != nil && !isAuthoritativeNoData(err6) {
			return RefreshResult{}, fmt.Errorf("resolve AAAA for %s: %w", host, err6)
		}
		usable := 0
		for _, addr := range addrs4 {
			if addr.Unmap().Is4() {
				usable++
			}
		}
		for _, addr := range addrs6 {
			prefix, ok := DeriveIPv6Prefix(addr, cfg.IPv6PrefixLen)
			if !ok {
				continue
			}
			usable++
			if _, seen := ipv6Seen[prefix]; !seen {
				ipv6Seen[prefix] = struct{}{}
				result.IPv6 = append(result.IPv6, prefix)
			}
		}
		if usable == 0 {
			return RefreshResult{}, fmt.Errorf("resolve %s: no usable A or AAAA records", host)
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

func StatusOf(ctx context.Context, cfg Config, resolver Resolver, runtime RuntimeSet, metadataPath ...string) Status {
	ensureDefaults(&cfg)
	status := Status{Enabled: cfg.Enabled, Hosts: append([]string(nil), cfg.Hosts...), IPv6PrefixLen: cfg.IPv6PrefixLen}
	if cfg.Enabled && resolver != nil && len(cfg.Hosts) > 0 {
		status.Configured, status.ResolutionError = Resolve(ctx, cfg, resolver)
	}
	if runtime != nil {
		status.Runtime, status.RuntimeError = runtime.List(ctx)
	}
	path := MetadataPath
	if len(metadataPath) > 0 && metadataPath[0] != "" {
		path = metadataPath[0]
	}
	status.Metadata, status.MetadataError = LoadMetadata(path)
	if status.MetadataError == nil && !status.Metadata.ExpiresAt.IsZero() {
		status.Stale = !time.Now().Before(status.Metadata.ExpiresAt)
	}
	return status
}

func RecordAttempt(path string, previous Metadata, result RefreshResult, ttl time.Duration, refreshErr error, now time.Time) (Metadata, error) {
	previous.Attempts++
	previous.LastAttempt = now.UTC()
	if refreshErr != nil {
		previous.ErrorCode = errorCode(refreshErr)
		previous.ErrorSummary = refreshErr.Error()
	} else {
		previous.LastSuccess = now.UTC()
		previous.ErrorCode, previous.ErrorSummary = "", ""
		previous.IPv4Count, previous.IPv6Count = len(result.IPv4), len(result.IPv6)
		previous.ContentHash = resultHash(result)
		previous.ExpiresAt = now.UTC().Add(ttl)
	}
	return previous, SaveMetadata(path, previous)
}

func LoadMetadata(path string) (Metadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Metadata{}, err
	}
	var metadata Metadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return Metadata{}, err
	}
	return metadata, nil
}

func SaveMetadata(path string, metadata Metadata) error {
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".ddns-runtime-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err = f.Write(data); err == nil {
		err = f.Chmod(0o600)
	}
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(tmp, path); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

func isAuthoritativeNoData(err error) bool {
	var addrErr *net.AddrError
	if errors.As(err, &addrErr) && addrErr.Err == "no suitable address found" {
		return true
	}
	var dnsErr *net.DNSError
	if !errors.As(err, &dnsErr) || dnsErr.IsTimeout || dnsErr.IsTemporary {
		return false
	}
	return dnsErr.IsNotFound || dnsErr.Err == "no suitable address found"
}

func errorCode(err error) string {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		if dnsErr.IsTimeout {
			return "dns_timeout"
		}
		if dnsErr.IsTemporary {
			return "dns_temporary"
		}
		if dnsErr.IsNotFound {
			return "dns_not_found"
		}
		return "dns_error"
	}
	return "refresh_error"
}

func resultHash(result RefreshResult) string {
	var values []string
	for _, addr := range result.IPv4 {
		values = append(values, "4:"+addr.String())
	}
	for _, prefix := range result.IPv6 {
		values = append(values, "6:"+prefix.String())
	}
	sort.Strings(values)
	sum := sha256.Sum256([]byte(strings.Join(values, "\n")))
	return hex.EncodeToString(sum[:])
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
