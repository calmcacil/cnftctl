package preset

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/calmcacil/cnftctl/internal/config"
)

type Preset struct {
	Config config.Config
}

func DecodeString(encoded string) (Preset, error) {
	encoded = strings.TrimSpace(encoded)
	b, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		b, err = base64.URLEncoding.DecodeString(encoded)
		if err != nil {
			return Preset{}, fmt.Errorf("decode preset: %w", err)
		}
	}
	return DecodeJSON(b)
}

func DecodeJSON(b []byte) (Preset, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	cfg := config.Default()
	if err := dec.Decode(&cfg); err != nil {
		return Preset{}, fmt.Errorf("decode preset json: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err != nil {
			return Preset{}, fmt.Errorf("decode preset json: %w", err)
		}
		return Preset{}, fmt.Errorf("decode preset json: trailing data")
	}
	if err := cfg.Validate(); err != nil {
		return Preset{}, err
	}
	return Preset{Config: cfg}, nil
}

func Encode(p Preset) (string, error) {
	if err := p.Validate(); err != nil {
		return "", err
	}
	b, err := json.Marshal(p.Config)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (p Preset) Validate() error {
	return p.Config.Validate()
}

func (p Preset) Explain() []string {
	var lines []string
	cfg := p.Config
	lines = append(lines, fmt.Sprintf("config version: %d", cfg.Version))
	if cfg.WANInterface == "" {
		lines = append(lines, "WAN interface: not preset")
	} else {
		lines = append(lines, "WAN interface: "+cfg.WANInterface)
	}
	lines = append(lines, fmt.Sprintf("open ports: %d", len(cfg.OpenPorts)))
	lines = append(lines, "SSH mode: "+cfg.SSH.Mode)
	lines = append(lines, fmt.Sprintf("static SSH whitelist entries: %d IPv4, %d IPv6", len(cfg.SSH.StaticWhitelist.IPv4), len(cfg.SSH.StaticWhitelist.IPv6)))
	lines = append(lines, fmt.Sprintf("DDNS whitelist: enabled=%t hosts=%d ttl=%s refresh_interval=%s ipv6_prefix_len=%d", cfg.SSH.DDNSWhitelist.Enabled, len(cfg.SSH.DDNSWhitelist.Hosts), cfg.SSH.DDNSWhitelist.TTL, cfg.SSH.DDNSWhitelist.RefreshInterval, cfg.SSH.DDNSWhitelist.IPv6PrefixLen))
	lines = append(lines, fmt.Sprintf("trusted interfaces: enabled=%t interfaces=%d trust_forwarding=%t", cfg.TrustedInterfaces.Enabled, len(cfg.TrustedInterfaces.Interfaces), cfg.TrustedInterfaces.TrustForwarding))
	lines = append(lines, fmt.Sprintf("Docker integration: enabled=%t allow_published_ports_by_default=%t", cfg.Docker.Enabled, cfg.Docker.AllowPublishedPortsByDefault))
	risks := cfg.RiskExplanations()
	if len(risks) == 0 {
		lines = append(lines, "risk warnings: none")
		return lines
	}
	sort.Strings(risks)
	lines = append(lines, "risk warnings:")
	for _, risk := range risks {
		lines = append(lines, "- "+risk)
	}
	return lines
}
