package features

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
)

const (
	SSHModeOpen               = "open"
	SSHModeWhitelistOnly      = "whitelist-only"
	SSHModeWhitelistRateLimit = "whitelist-rate-limit"

	WarningSSHHardened        = "ssh_hardened_apply_required"
	WarningTrustedFullTrust   = "trusted_interface_full_trust"
	WarningDockerPublishedWAN = "docker_published_ports_gated_by_open_ports"
)

var interfaceNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,15}$`)

type Config struct {
	SSH               SSHConfig
	TrustedInterfaces TrustedInterfacesConfig
	Docker            DockerConfig
}

type SSHConfig struct {
	Mode              string
	StaticWhitelist   []string
	DDNSEnabled       bool
	DDNSHosts         []string
	HardeningOverride bool
}

type TrustedInterfacesConfig struct {
	Enabled    bool
	Interfaces []string
}

type DockerConfig struct {
	Enabled bool
}

type Warning struct {
	Code    string
	Message string
}

type Result struct {
	Changed  bool
	Warnings []Warning
}

func SetSSHMode(cfg *Config, mode string, override bool) (Result, error) {
	if cfg == nil {
		return Result{}, errors.New("features config is nil")
	}
	if mode != SSHModeOpen && mode != SSHModeWhitelistOnly && mode != SSHModeWhitelistRateLimit {
		return Result{}, fmt.Errorf("unsupported SSH mode %q", mode)
	}
	if mode != SSHModeOpen && !override && !hasWhitelistCoverage(cfg.SSH) {
		return Result{}, errors.New("SSH hardening requires static or DDNS whitelist coverage unless override is set")
	}

	changed := cfg.SSH.Mode != mode || cfg.SSH.HardeningOverride != override
	cfg.SSH.Mode = mode
	cfg.SSH.HardeningOverride = override

	var warnings []Warning
	if mode != SSHModeOpen {
		warnings = append(warnings, Warning{Code: WarningSSHHardened, Message: "SSH hardening changes require apply and can lock you out if whitelist coverage is wrong."})
	}
	return Result{Changed: changed, Warnings: warnings}, nil
}

func EnableTrustedInterface(cfg *Config, name string) (Result, error) {
	if cfg == nil {
		return Result{}, errors.New("features config is nil")
	}
	if !interfaceNamePattern.MatchString(name) {
		return Result{}, fmt.Errorf("invalid trusted interface name %q", name)
	}

	for _, existing := range cfg.TrustedInterfaces.Interfaces {
		if existing == name {
			cfg.TrustedInterfaces.Enabled = true
			return Result{Warnings: trustedWarnings(name)}, nil
		}
	}
	cfg.TrustedInterfaces.Interfaces = append(cfg.TrustedInterfaces.Interfaces, name)
	sort.Strings(cfg.TrustedInterfaces.Interfaces)
	cfg.TrustedInterfaces.Enabled = true
	return Result{Changed: true, Warnings: trustedWarnings(name)}, nil
}

func DisableTrustedInterface(cfg *Config, name string) (Result, error) {
	if cfg == nil {
		return Result{}, errors.New("features config is nil")
	}
	if !interfaceNamePattern.MatchString(name) {
		return Result{}, fmt.Errorf("invalid trusted interface name %q", name)
	}

	for i, existing := range cfg.TrustedInterfaces.Interfaces {
		if existing == name {
			cfg.TrustedInterfaces.Interfaces = append(cfg.TrustedInterfaces.Interfaces[:i], cfg.TrustedInterfaces.Interfaces[i+1:]...)
			cfg.TrustedInterfaces.Enabled = len(cfg.TrustedInterfaces.Interfaces) > 0
			return Result{Changed: true}, nil
		}
	}
	return Result{}, nil
}

func EnableDocker(cfg *Config) (Result, error) {
	if cfg == nil {
		return Result{}, errors.New("features config is nil")
	}
	changed := !cfg.Docker.Enabled
	cfg.Docker.Enabled = true
	return Result{Changed: changed, Warnings: []Warning{{Code: WarningDockerPublishedWAN, Message: "Docker-published ports remain blocked from WAN until the matching protocol/port is added to open ports."}}}, nil
}

func DisableDocker(cfg *Config) (Result, error) {
	if cfg == nil {
		return Result{}, errors.New("features config is nil")
	}
	changed := cfg.Docker.Enabled
	cfg.Docker.Enabled = false
	return Result{Changed: changed}, nil
}

func hasWhitelistCoverage(cfg SSHConfig) bool {
	return len(cfg.StaticWhitelist) > 0 || (cfg.DDNSEnabled && len(cfg.DDNSHosts) > 0)
}

func trustedWarnings(name string) []Warning {
	return []Warning{{Code: WarningTrustedFullTrust, Message: fmt.Sprintf("Traffic arriving on %s will be fully trusted by the firewall profile.", name)}}
}
