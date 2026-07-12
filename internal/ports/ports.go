package ports

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

const WarningDockerExposure = "docker_open_port_exposure"

type Entry struct {
	Protocol string
	Start    uint16
	End      uint16
	Comment  string
}

type Config struct {
	OpenPorts     []Entry
	DockerEnabled bool
}

type Warning struct {
	Code    string
	Message string
}

type Result struct {
	Changed  bool
	Entry    Entry
	Entries  []Entry
	Warnings []Warning
}

func Open(cfg *Config, protocol, portSpec, comment string) (Result, error) {
	if cfg == nil {
		return Result{}, errors.New("ports config is nil")
	}

	entry, err := ParseEntry(protocol, portSpec, comment)
	if err != nil {
		return Result{}, err
	}

	for _, existing := range cfg.OpenPorts {
		if samePort(existing, entry) {
			return Result{Entry: existing, Entries: List(cfg), Warnings: dockerWarnings(cfg)}, nil
		}
	}

	cfg.OpenPorts = append(cfg.OpenPorts, entry)
	return Result{Changed: true, Entry: entry, Entries: List(cfg), Warnings: dockerWarnings(cfg)}, nil
}

func Close(cfg *Config, protocol, portSpec string, strict bool) (Result, error) {
	if cfg == nil {
		return Result{}, errors.New("ports config is nil")
	}

	entry, err := ParseEntry(protocol, portSpec, "")
	if err != nil {
		return Result{}, err
	}

	for i, existing := range cfg.OpenPorts {
		if samePort(existing, entry) {
			cfg.OpenPorts = append(cfg.OpenPorts[:i], cfg.OpenPorts[i+1:]...)
			return Result{Changed: true, Entry: existing, Entries: List(cfg), Warnings: dockerWarnings(cfg)}, nil
		}
	}

	if strict {
		return Result{}, fmt.Errorf("%s %s is not open", entry.Protocol, FormatPort(entry))
	}
	return Result{Entry: entry, Entries: List(cfg), Warnings: dockerWarnings(cfg)}, nil
}

func List(cfg *Config) []Entry {
	if cfg == nil || len(cfg.OpenPorts) == 0 {
		return nil
	}

	entries := append([]Entry(nil), cfg.OpenPorts...)
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Protocol != entries[j].Protocol {
			return entries[i].Protocol < entries[j].Protocol
		}
		if entries[i].Start != entries[j].Start {
			return entries[i].Start < entries[j].Start
		}
		return entries[i].End < entries[j].End
	})
	return entries
}

func ParseEntry(protocol, portSpec, comment string) (Entry, error) {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	if protocol != "tcp" && protocol != "udp" {
		return Entry{}, fmt.Errorf("unsupported protocol %q: must be tcp or udp", protocol)
	}

	start, end, err := parsePortSpec(portSpec)
	if err != nil {
		return Entry{}, err
	}

	comment = strings.TrimSpace(comment)
	if strings.IndexFunc(comment, unicode.IsControl) >= 0 {
		return Entry{}, errors.New("comment must not contain control characters")
	}
	return Entry{Protocol: protocol, Start: start, End: end, Comment: comment}, nil
}

func FormatPort(entry Entry) string {
	if entry.Start == entry.End {
		return strconv.Itoa(int(entry.Start))
	}
	return fmt.Sprintf("%d-%d", entry.Start, entry.End)
}

func parsePortSpec(spec string) (uint16, uint16, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return 0, 0, errors.New("port is required")
	}

	parts := strings.Split(spec, "-")
	if len(parts) > 2 {
		return 0, 0, fmt.Errorf("invalid port range %q", spec)
	}

	start, err := parsePort(parts[0])
	if err != nil {
		return 0, 0, err
	}
	end := start
	if len(parts) == 2 {
		end, err = parsePort(parts[1])
		if err != nil {
			return 0, 0, err
		}
		if end < start {
			return 0, 0, fmt.Errorf("invalid port range %q: end is before start", spec)
		}
	}
	return start, end, nil
}

func parsePort(value string) (uint16, error) {
	value = strings.TrimSpace(value)
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("invalid port %q: must be 1..65535", value)
	}
	return uint16(port), nil
}

func samePort(a, b Entry) bool {
	return a.Protocol == b.Protocol && a.Start == b.Start && a.End == b.End
}

func dockerWarnings(cfg *Config) []Warning {
	if cfg == nil || !cfg.DockerEnabled {
		return nil
	}
	return []Warning{{
		Code:    WarningDockerExposure,
		Message: "Docker integration is enabled; open ports are public from WAN for both host services and Docker-published services.",
	}}
}
