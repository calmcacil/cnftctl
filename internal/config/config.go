package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"gopkg.in/yaml.v3"
)

const CurrentVersion = 1

var (
	validInterfaceName = regexp.MustCompile(`^[A-Za-z0-9_.:-]+$`)
	validDockerIface   = regexp.MustCompile(`^[A-Za-z0-9_.:-]+\*?$`)
)

type Config struct {
	Version           int               `json:"version" yaml:"version"`
	WANInterface      string            `json:"wan_interface" yaml:"wan_interface"`
	OpenPorts         []OpenPort        `json:"open_ports" yaml:"open_ports"`
	SSH               SSH               `json:"ssh" yaml:"ssh"`
	TrustedInterfaces TrustedInterfaces `json:"trusted_interfaces" yaml:"trusted_interfaces"`
	Docker            Docker            `json:"docker" yaml:"docker"`
}

type OpenPort struct {
	Protocol string `json:"protocol" yaml:"protocol"`
	Port     int    `json:"port" yaml:"port"`
	EndPort  int    `json:"end_port,omitempty" yaml:"end_port,omitempty"`
	Comment  string `json:"comment,omitempty" yaml:"comment,omitempty"`
}

type SSH struct {
	Mode            string        `json:"mode" yaml:"mode"`
	RateLimit       *RateLimit    `json:"rate_limit" yaml:"rate_limit"`
	StaticWhitelist IPWhitelist   `json:"static_whitelist" yaml:"static_whitelist"`
	DDNSWhitelist   DDNSWhitelist `json:"ddns_whitelist" yaml:"ddns_whitelist"`
}

type RateLimit struct {
	Connections int      `json:"connections" yaml:"connections"`
	Per         Duration `json:"per" yaml:"per"`
}

type IPWhitelist struct {
	IPv4 []WhitelistEntry `json:"ipv4" yaml:"ipv4"`
	IPv6 []WhitelistEntry `json:"ipv6" yaml:"ipv6"`
}

type WhitelistEntry struct {
	Value   string `json:"value" yaml:"value"`
	Comment string `json:"comment,omitempty" yaml:"comment,omitempty"`
}

type DDNSWhitelist struct {
	Enabled         bool     `json:"enabled" yaml:"enabled"`
	Hosts           []string `json:"hosts" yaml:"hosts"`
	TTL             Duration `json:"ttl" yaml:"ttl"`
	RefreshInterval Duration `json:"refresh_interval" yaml:"refresh_interval"`
	IPv6PrefixLen   int      `json:"ipv6_prefix_len" yaml:"ipv6_prefix_len"`
}

type TrustedInterfaces struct {
	Enabled         bool     `json:"enabled" yaml:"enabled"`
	Interfaces      []string `json:"interfaces" yaml:"interfaces"`
	TrustForwarding bool     `json:"trust_forwarding" yaml:"trust_forwarding"`
}

type Docker struct {
	Enabled    bool     `json:"enabled" yaml:"enabled"`
	Interfaces []string `json:"interfaces" yaml:"interfaces"`
}

type Duration struct {
	time.Duration
}

func Default() Config {
	return Config{
		Version:   CurrentVersion,
		OpenPorts: []OpenPort{},
		SSH: SSH{
			Mode:      "open",
			RateLimit: nil,
			StaticWhitelist: IPWhitelist{
				IPv4: []WhitelistEntry{},
				IPv6: []WhitelistEntry{},
			},
			DDNSWhitelist: DDNSWhitelist{
				Enabled:         false,
				Hosts:           []string{},
				TTL:             Duration{Duration: time.Hour},
				RefreshInterval: Duration{Duration: 5 * time.Minute},
				IPv6PrefixLen:   56,
			},
		},
		TrustedInterfaces: TrustedInterfaces{
			Enabled:         false,
			Interfaces:      []string{},
			TrustForwarding: false,
		},
		Docker: Docker{
			Enabled:    false,
			Interfaces: []string{"docker0", "br-*"},
		},
	}
}

func Load(r io.Reader) (Config, error) {
	cfg := Default()
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, err
	}
	cfg.canonicalize()
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func LoadFile(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer f.Close()
	return Load(f)
}

func Save(w io.Writer, cfg Config) error {
	cfg.canonicalize()
	if err := cfg.Validate(); err != nil {
		return err
	}
	b, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

func SaveFile(path string, cfg Config, perm os.FileMode) error {
	var data bytes.Buffer
	if err := Save(&data, cfg); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".config-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err = tmp.Write(data.Bytes()); err == nil {
		err = tmp.Chmod(perm)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	parent, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer parent.Close()
	return parent.Sync()
}

func (c Config) Validate() error {
	var errs ValidationErrors
	if c.Version != CurrentVersion {
		errs.add("version", fmt.Sprintf("unsupported config version %d", c.Version))
	}
	if c.WANInterface != "" && !isInterfaceName(c.WANInterface) {
		errs.add("wan_interface", "must be a valid Linux interface name")
	}
	for i, p := range c.OpenPorts {
		path := fmt.Sprintf("open_ports[%d]", i)
		if !validProtocol(p.Protocol) {
			errs.add(path+".protocol", "must be tcp or udp")
		}
		if !validPort(p.Port) {
			errs.add(path+".port", "must be in range 1..65535")
		}
		if p.EndPort != 0 {
			if !validPort(p.EndPort) {
				errs.add(path+".end_port", "must be in range 1..65535")
			} else if p.Port != 0 && p.EndPort < p.Port {
				errs.add(path+".end_port", "must be greater than or equal to port")
			}
		}
		if hasControl(p.Comment) {
			errs.add(path+".comment", "must not contain control characters")
		}
	}
	validateOpenPortDuplicates(c.OpenPorts, &errs)
	validateSSH(c.SSH, &errs)
	validateTrusted(c.TrustedInterfaces, &errs)
	validateDocker(c.Docker, &errs)
	if len(errs) > 0 {
		return errs
	}
	return nil
}

func (c Config) RiskExplanations() []string {
	var risks []string
	for _, p := range c.OpenPorts {
		rangeText := fmt.Sprintf("%d", p.Port)
		if p.EndPort != 0 && p.EndPort != p.Port {
			rangeText = fmt.Sprintf("%d-%d", p.Port, p.EndPort)
		}
		risks = append(risks, fmt.Sprintf("public %s port %s will be reachable from WAN for host services", p.Protocol, rangeText))
	}
	if c.SSH.Mode == "whitelist-only" {
		risks = append(risks, "SSH hardening mode whitelist-only can lock you out unless the whitelist covers your current access path")
	}
	if c.SSH.Mode == "whitelist-rate-limit" {
		risks = append(risks, "SSH hardening mode whitelist-rate-limit can still lock you out if whitelist or rate-limit settings are wrong")
	}
	for _, entry := range append(append([]WhitelistEntry{}, c.SSH.StaticWhitelist.IPv4...), c.SSH.StaticWhitelist.IPv6...) {
		if isBroadPrefix(entry.Value) {
			risks = append(risks, fmt.Sprintf("broad SSH static whitelist prefix %s grants access to many source addresses", entry.Value))
		}
	}
	if c.SSH.DDNSWhitelist.Enabled {
		risks = append(risks, "DDNS SSH whitelist trusts DNS results for configured hostnames")
	}
	if c.TrustedInterfaces.Enabled {
		risks = append(risks, "trusted interfaces grant full trust to traffic arriving on configured overlay/VPN interfaces")
	}
	if c.TrustedInterfaces.TrustForwarding {
		risks = append(risks, "trusted interface forwarding extends trust beyond host input traffic")
	}
	if c.Docker.Enabled {
		risks = append(risks, "Docker integration exposes Docker-published services from WAN when their protocol and port are listed in open_ports")
	}
	sort.Strings(risks)
	return risks
}

type ValidationErrors []ValidationError

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationErrors) add(field, message string) {
	*e = append(*e, ValidationError{Field: field, Message: message})
}

func (e ValidationErrors) Error() string {
	parts := make([]string, 0, len(e))
	for _, err := range e {
		parts = append(parts, err.Field+": "+err.Message)
	}
	return strings.Join(parts, "; ")
}

func (e ValidationErrors) Is(target error) bool {
	_, ok := target.(ValidationErrors)
	return ok
}

func validateSSH(ssh SSH, errs *ValidationErrors) {
	switch ssh.Mode {
	case "open", "whitelist-only", "whitelist-rate-limit":
	default:
		errs.add("ssh.mode", "must be open, whitelist-only, or whitelist-rate-limit")
	}
	if ssh.RateLimit != nil {
		if ssh.RateLimit.Connections <= 0 {
			errs.add("ssh.rate_limit.connections", "must be greater than zero")
		}
		if per := ssh.RateLimit.Per.Duration; per != time.Second && per != time.Minute && per != time.Hour {
			errs.add("ssh.rate_limit.per", "must be exactly 1s, 1m, or 1h")
		}
	}
	if ssh.Mode == "whitelist-rate-limit" && ssh.RateLimit == nil {
		errs.add("ssh.rate_limit", "is required for whitelist-rate-limit mode")
	}
	if ssh.Mode != "whitelist-rate-limit" && ssh.RateLimit != nil {
		errs.add("ssh.rate_limit", "is only valid for whitelist-rate-limit mode")
	}
	if ssh.Mode != "open" && len(ssh.StaticWhitelist.IPv4)+len(ssh.StaticWhitelist.IPv6) == 0 && !(ssh.DDNSWhitelist.Enabled && len(ssh.DDNSWhitelist.Hosts) > 0) {
		errs.add("ssh.mode", "hardened SSH requires an effective static or enabled DDNS whitelist")
	}
	validateIPList("ssh.static_whitelist.ipv4", ssh.StaticWhitelist.IPv4, false, errs)
	validateIPList("ssh.static_whitelist.ipv6", ssh.StaticWhitelist.IPv6, true, errs)
	for i, host := range ssh.DDNSWhitelist.Hosts {
		if !isHostname(host) {
			errs.add(fmt.Sprintf("ssh.ddns_whitelist.hosts[%d]", i), "must be a valid hostname")
		}
	}
	validateStringDuplicates("ssh.ddns_whitelist.hosts", ssh.DDNSWhitelist.Hosts, strings.ToLower, errs)
	if ssh.DDNSWhitelist.Enabled && len(ssh.DDNSWhitelist.Hosts) == 0 {
		errs.add("ssh.ddns_whitelist.hosts", "must not be empty when DDNS whitelist is enabled")
	}
	if ssh.DDNSWhitelist.TTL.Duration <= 0 {
		errs.add("ssh.ddns_whitelist.ttl", "must be greater than zero")
	}
	if ssh.DDNSWhitelist.RefreshInterval.Duration <= 0 {
		errs.add("ssh.ddns_whitelist.refresh_interval", "must be greater than zero")
	}
	if ssh.DDNSWhitelist.IPv6PrefixLen != 56 && ssh.DDNSWhitelist.IPv6PrefixLen != 64 {
		errs.add("ssh.ddns_whitelist.ipv6_prefix_len", "must be 56 or 64")
	}
}

func validateTrusted(t TrustedInterfaces, errs *ValidationErrors) {
	for i, iface := range t.Interfaces {
		if !isInterfaceName(iface) {
			errs.add(fmt.Sprintf("trusted_interfaces.interfaces[%d]", i), "must be a valid Linux interface name")
		}
	}
	validateStringDuplicates("trusted_interfaces.interfaces", t.Interfaces, func(s string) string { return s }, errs)
	if t.Enabled && len(t.Interfaces) == 0 {
		errs.add("trusted_interfaces.interfaces", "must not be empty when trusted interfaces are enabled")
	}
	if t.TrustForwarding && !t.Enabled {
		errs.add("trusted_interfaces.trust_forwarding", "requires trusted interfaces to be enabled")
	}
}

func validateDocker(d Docker, errs *ValidationErrors) {
	for i, iface := range d.Interfaces {
		if !isDockerInterfacePattern(iface) {
			errs.add(fmt.Sprintf("docker.interfaces[%d]", i), "must be a valid interface name or trailing-* wildcard pattern")
		}
	}
	validateStringDuplicates("docker.interfaces", d.Interfaces, func(s string) string { return s }, errs)
	if d.Enabled && len(d.Interfaces) == 0 {
		errs.add("docker.interfaces", "must not be empty when Docker integration is enabled")
	}
}

func validateIPList(field string, entries []WhitelistEntry, wantIPv6 bool, errs *ValidationErrors) {
	seen := map[string]int{}
	for i, entry := range entries {
		addr, prefix, isPrefix, err := parseAddrOrPrefix(entry.Value)
		if err != nil {
			errs.add(fmt.Sprintf("%s[%d]", field, i), "must be a valid IP address or CIDR prefix")
			continue
		}
		is6 := addr.Is6()
		if isPrefix {
			is6 = prefix.Addr().Is6()
		}
		if wantIPv6 != is6 {
			errs.add(fmt.Sprintf("%s[%d]", field, i), "must match the whitelist IP family")
		}
		if hasControl(entry.Comment) {
			errs.add(fmt.Sprintf("%s[%d].comment", field, i), "must not contain control characters")
		}
		canonical := addr.String()
		if isPrefix {
			canonical = prefix.Masked().String()
		}
		if first, ok := seen[canonical]; ok {
			errs.add(fmt.Sprintf("%s[%d].value", field, i), fmt.Sprintf("duplicates %s[%d]", field, first))
		} else {
			seen[canonical] = i
		}
	}
}

func validateOpenPortDuplicates(entries []OpenPort, errs *ValidationErrors) {
	seen := map[string]int{}
	for i, entry := range entries {
		end := entry.EndPort
		if end == 0 {
			end = entry.Port
		}
		key := fmt.Sprintf("%s/%d/%d", strings.ToLower(entry.Protocol), entry.Port, end)
		if first, ok := seen[key]; ok {
			errs.add(fmt.Sprintf("open_ports[%d]", i), fmt.Sprintf("duplicates open_ports[%d]", first))
		} else {
			seen[key] = i
		}
	}
}

func validateStringDuplicates(field string, values []string, canonical func(string) string, errs *ValidationErrors) {
	seen := map[string]int{}
	for i, value := range values {
		key := canonical(value)
		if first, ok := seen[key]; ok {
			errs.add(fmt.Sprintf("%s[%d]", field, i), fmt.Sprintf("duplicates %s[%d]", field, first))
		} else {
			seen[key] = i
		}
	}
}

func (c *Config) canonicalize() {
	for i := range c.OpenPorts {
		c.OpenPorts[i].Protocol = strings.ToLower(c.OpenPorts[i].Protocol)
		c.OpenPorts[i].Comment = strings.TrimSpace(c.OpenPorts[i].Comment)
	}
	canonicalizeWhitelist := func(entries []WhitelistEntry) {
		for i := range entries {
			entries[i].Value = canonicalIP(entries[i].Value)
			entries[i].Comment = strings.TrimSpace(entries[i].Comment)
		}
	}
	canonicalizeWhitelist(c.SSH.StaticWhitelist.IPv4)
	canonicalizeWhitelist(c.SSH.StaticWhitelist.IPv6)
	for i := range c.SSH.DDNSWhitelist.Hosts {
		c.SSH.DDNSWhitelist.Hosts[i] = strings.ToLower(strings.TrimSuffix(c.SSH.DDNSWhitelist.Hosts[i], "."))
	}
}

func canonicalIP(value string) string {
	addr, prefix, isPrefix, err := parseAddrOrPrefix(value)
	if err != nil {
		return value
	}
	if isPrefix {
		return prefix.Masked().String()
	}
	return addr.String()
}

func hasControl(value string) bool {
	return strings.IndexFunc(value, unicode.IsControl) >= 0
}

func parseAddrOrPrefix(s string) (netip.Addr, netip.Prefix, bool, error) {
	if strings.Contains(s, "/") {
		prefix, err := netip.ParsePrefix(s)
		if err != nil {
			return netip.Addr{}, netip.Prefix{}, false, err
		}
		return prefix.Addr(), prefix, true, nil
	}
	addr, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Addr{}, netip.Prefix{}, false, err
	}
	return addr, netip.Prefix{}, false, nil
}

func validProtocol(protocol string) bool {
	return protocol == "tcp" || protocol == "udp"
}

func validPort(port int) bool {
	return port >= 1 && port <= 65535
}

func isInterfaceName(name string) bool {
	return name != "" && len(name) <= 15 && validInterfaceName.MatchString(name)
}

func isDockerInterfacePattern(name string) bool {
	if name == "" || len(name) > 15 || !validDockerIface.MatchString(name) {
		return false
	}
	return strings.Count(name, "*") <= 1 && (!strings.Contains(name, "*") || strings.HasSuffix(name, "*"))
}

func isHostname(host string) bool {
	host = strings.TrimSuffix(host, ".")
	if host == "" || len(host) > 253 || strings.Contains(host, "..") {
		return false
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return false
	}
	labels := strings.Split(host, ".")
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 {
			return false
		}
		for i, r := range label {
			ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-'
			if !ok || (r == '-' && (i == 0 || i == len(label)-1)) {
				return false
			}
		}
	}
	return true
}

func isBroadPrefix(entry string) bool {
	prefix, err := netip.ParsePrefix(entry)
	if err != nil {
		return false
	}
	if prefix.Addr().Is4() {
		return prefix.Bits() <= 24
	}
	return prefix.Bits() <= 64
}

func (d Duration) MarshalYAML() (any, error) {
	return d.String(), nil
}

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.ScalarNode {
		return errors.New("duration must be a scalar")
	}
	parsed, err := time.ParseDuration(value.Value)
	if err != nil {
		return err
	}
	d.Duration = parsed
	return nil
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("%q", d.String())), nil
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), "\"")
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	d.Duration = parsed
	return nil
}
