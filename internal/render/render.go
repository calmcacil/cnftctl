package render

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"text/template"
	"time"
)

const (
	NftablesConfPath = "/etc/nftables.conf"
	FragmentDir      = "/etc/cnftctl/nftables.d"
	OpenPortsPath    = FragmentDir + "/open-ports.nft"
	WhitelistPath    = FragmentDir + "/whitelist.nft"
	DDNSHostsPath    = FragmentDir + "/ddns-hosts.conf"
	DDNSServicePath  = "/etc/systemd/system/nft-ddns-whitelist.service"
	DDNSTimerPath    = "/etc/systemd/system/nft-ddns-whitelist.timer"
)

type SSHMode string

const (
	SSHOpen               SSHMode = "open"
	SSHWhitelistOnly      SSHMode = "whitelist-only"
	SSHWhitelistRateLimit SSHMode = "whitelist-rate-limit"
)

type Config struct {
	WANInterface      string
	OpenPorts         []OpenPort
	SSH               SSHConfig
	TrustedInterfaces TrustedInterfacesConfig
	Docker            DockerConfig
}

type OpenPort struct {
	Protocol string
	Port     string
	Comment  string
}

type SSHConfig struct {
	Mode            SSHMode
	RateLimit       string
	StaticWhitelist StaticWhitelist
	DDNSWhitelist   DDNSWhitelist
}

type StaticWhitelist struct {
	IPv4 []string
	IPv6 []string
}

type DDNSWhitelist struct {
	Enabled         bool
	Hosts           []string
	TTL             time.Duration
	RefreshInterval time.Duration
	IPv6PrefixLen   int
}

type TrustedInterfacesConfig struct {
	Enabled         bool
	Interfaces      []string
	TrustForwarding bool
}

type DockerConfig struct {
	Enabled    bool
	Interfaces []string
}

type File struct {
	Path    string
	Content string
}

func Files(cfg Config) ([]File, error) {
	model := newModel(cfg)

	files := []File{
		{Path: NftablesConfPath},
		{Path: OpenPortsPath},
		{Path: WhitelistPath},
	}

	var err error
	files[0].Content, err = renderTemplate("nftables.conf", nftablesTemplate, model)
	if err != nil {
		return nil, err
	}
	files[1].Content, err = renderTemplate("open-ports.nft", openPortsTemplate, model)
	if err != nil {
		return nil, err
	}
	files[2].Content, err = renderTemplate("whitelist.nft", whitelistTemplate, model)
	if err != nil {
		return nil, err
	}

	if model.DDNSEnabled {
		ddnsFiles := []File{
			{Path: DDNSHostsPath},
			{Path: DDNSServicePath},
			{Path: DDNSTimerPath},
		}
		ddnsFiles[0].Content, err = renderTemplate("ddns-hosts.conf", ddnsHostsTemplate, model)
		if err != nil {
			return nil, err
		}
		ddnsFiles[1].Content, err = renderTemplate("nft-ddns-whitelist.service", ddnsServiceTemplate, model)
		if err != nil {
			return nil, err
		}
		ddnsFiles[2].Content, err = renderTemplate("nft-ddns-whitelist.timer", ddnsTimerTemplate, model)
		if err != nil {
			return nil, err
		}
		files = append(files, ddnsFiles...)
	}

	return files, nil
}

func NftablesConf(cfg Config) (string, error) {
	return renderTemplate("nftables.conf", nftablesTemplate, newModel(cfg))
}

func OpenPorts(cfg Config) (string, error) {
	return renderTemplate("open-ports.nft", openPortsTemplate, newModel(cfg))
}

func Whitelist(cfg Config) (string, error) {
	return renderTemplate("whitelist.nft", whitelistTemplate, newModel(cfg))
}

type model struct {
	WANInterface      string
	OpenPorts         []OpenPort
	SSHMode           SSHMode
	SSHRateLimit      string
	WhitelistIPv4     []string
	WhitelistIPv6     []string
	DDNSEnabled       bool
	DDNSTTL           string
	DDNSRefresh       string
	DDNSHosts         []string
	TrustedEnabled    bool
	TrustedInterfaces []string
	TrustForwarding   bool
	DockerEnabled     bool
	DockerInterfaces  []string
}

func newModel(cfg Config) model {
	sshMode := cfg.SSH.Mode
	if sshMode == "" {
		sshMode = SSHOpen
	}

	wanIf := cfg.WANInterface
	if wanIf == "" {
		wanIf = "eth0"
	}

	openPorts := append([]OpenPort(nil), cfg.OpenPorts...)
	sort.SliceStable(openPorts, func(i, j int) bool {
		if openPorts[i].Protocol != openPorts[j].Protocol {
			return openPorts[i].Protocol < openPorts[j].Protocol
		}
		return openPorts[i].Port < openPorts[j].Port
	})

	whitelistV4 := sortedStrings(cfg.SSH.StaticWhitelist.IPv4)
	whitelistV6 := sortedStrings(cfg.SSH.StaticWhitelist.IPv6)
	trustedIfs := sortedStrings(cfg.TrustedInterfaces.Interfaces)
	dockerIfs := sortedStrings(cfg.Docker.Interfaces)
	if cfg.Docker.Enabled && len(dockerIfs) == 0 {
		dockerIfs = []string{"br-*", "docker0"}
	}

	ttl := cfg.SSH.DDNSWhitelist.TTL
	if ttl == 0 {
		ttl = time.Hour
	}
	refresh := cfg.SSH.DDNSWhitelist.RefreshInterval
	if refresh == 0 {
		refresh = 5 * time.Minute
	}

	return model{
		WANInterface:      wanIf,
		OpenPorts:         openPorts,
		SSHMode:           sshMode,
		SSHRateLimit:      rateLimit(cfg.SSH.RateLimit),
		WhitelistIPv4:     whitelistV4,
		WhitelistIPv6:     whitelistV6,
		DDNSEnabled:       cfg.SSH.DDNSWhitelist.Enabled,
		DDNSTTL:           nftDuration(ttl),
		DDNSRefresh:       systemdDuration(refresh),
		DDNSHosts:         sortedStrings(cfg.SSH.DDNSWhitelist.Hosts),
		TrustedEnabled:    cfg.TrustedInterfaces.Enabled && len(trustedIfs) > 0,
		TrustedInterfaces: trustedIfs,
		TrustForwarding:   cfg.TrustedInterfaces.Enabled && cfg.TrustedInterfaces.TrustForwarding && len(trustedIfs) > 0,
		DockerEnabled:     cfg.Docker.Enabled,
		DockerInterfaces:  dockerIfs,
	}
}

func renderTemplate(name, text string, data model) (string, error) {
	tmpl, err := template.New(name).Funcs(template.FuncMap{
		"quoteSet": quoteSet,
		"nftSet":   nftSet,
	}).Parse(text)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func sortedStrings(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}

func quoteSet(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, fmt.Sprintf("%q", value))
	}
	return "{ " + strings.Join(quoted, ", ") + " }"
}

func nftSet(values []string) string {
	if len(values) == 0 {
		return "{ }"
	}
	return "{ " + strings.Join(values, ", ") + " }"
}

func rateLimit(value string) string {
	if value == "" {
		return "6/minute burst 3 packets"
	}
	return value
}

func nftDuration(d time.Duration) string {
	if d%time.Hour == 0 {
		return fmt.Sprintf("%dh", int(d/time.Hour))
	}
	if d%time.Minute == 0 {
		return fmt.Sprintf("%dm", int(d/time.Minute))
	}
	return fmt.Sprintf("%ds", int(d/time.Second))
}

func systemdDuration(d time.Duration) string {
	if d%time.Hour == 0 {
		return fmt.Sprintf("%dh", int(d/time.Hour))
	}
	if d%time.Minute == 0 {
		return fmt.Sprintf("%dm", int(d/time.Minute))
	}
	return fmt.Sprintf("%ds", int(d/time.Second))
}
