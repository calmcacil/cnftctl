package config

import (
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

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
	IPv4 []string `json:"ipv4" yaml:"ipv4"`
	IPv6 []string `json:"ipv6" yaml:"ipv6"`
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
	Enabled                      bool     `json:"enabled" yaml:"enabled"`
	AllowPublishedPortsByDefault bool     `json:"allow_published_ports_by_default" yaml:"allow_published_ports_by_default"`
	Interfaces                   []string `json:"interfaces" yaml:"interfaces"`
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
				IPv4: []string{},
				IPv6: []string{},
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
			Enabled:                      false,
			AllowPublishedPortsByDefault: false,
			Interfaces:                   []string{"docker0", "br-*"},
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
	if err := cfg.Validate(); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer f.Close()
	return Save(f, cfg)
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
	}
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
	for _, entry := range append(append([]string{}, c.SSH.StaticWhitelist.IPv4...), c.SSH.StaticWhitelist.IPv6...) {
		if isBroadPrefix(entry) {
			risks = append(risks, fmt.Sprintf("broad SSH static whitelist prefix %s grants access to many source addresses", entry))
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
	if c.Docker.AllowPublishedPortsByDefault {
		risks = append(risks, "allow_published_ports_by_default can expose Docker-published services without matching open_ports entries")
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
		if ssh.RateLimit.Per.Duration <= 0 {
			errs.add("ssh.rate_limit.per", "must be greater than zero")
		}
	}
	validateIPList("ssh.static_whitelist.ipv4", ssh.StaticWhitelist.IPv4, false, errs)
	validateIPList("ssh.static_whitelist.ipv6", ssh.StaticWhitelist.IPv6, true, errs)
	for i, host := range ssh.DDNSWhitelist.Hosts {
		if !isHostname(host) {
			errs.add(fmt.Sprintf("ssh.ddns_whitelist.hosts[%d]", i), "must be a valid hostname")
		}
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
}

func validateDocker(d Docker, errs *ValidationErrors) {
	for i, iface := range d.Interfaces {
		if !isDockerInterfacePattern(iface) {
			errs.add(fmt.Sprintf("docker.interfaces[%d]", i), "must be a valid interface name or trailing-* wildcard pattern")
		}
	}
}

func validateIPList(field string, entries []string, wantIPv6 bool, errs *ValidationErrors) {
	for i, entry := range entries {
		addr, prefix, isPrefix, err := parseAddrOrPrefix(entry)
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
	}
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
