package whitelist

import (
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"unicode"
)

const WarningBroadPrefix = "broad_ssh_whitelist_prefix"

type Entry struct {
	Prefix  netip.Prefix
	Comment string
}

type Config struct {
	Static []Entry
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

func Add(cfg *Config, value, comment string) (Result, error) {
	if cfg == nil {
		return Result{}, errors.New("whitelist config is nil")
	}

	entry, err := ParseEntry(value, comment)
	if err != nil {
		return Result{}, err
	}

	for _, existing := range cfg.Static {
		if existing.Prefix == entry.Prefix {
			return Result{Entry: existing, Entries: List(cfg), Warnings: broadWarnings(entry)}, nil
		}
	}

	cfg.Static = append(cfg.Static, entry)
	return Result{Changed: true, Entry: entry, Entries: List(cfg), Warnings: broadWarnings(entry)}, nil
}

func Remove(cfg *Config, value string) (Result, error) {
	if cfg == nil {
		return Result{}, errors.New("whitelist config is nil")
	}

	entry, err := ParseEntry(value, "")
	if err != nil {
		return Result{}, err
	}

	for i, existing := range cfg.Static {
		if existing.Prefix == entry.Prefix {
			cfg.Static = append(cfg.Static[:i], cfg.Static[i+1:]...)
			return Result{Changed: true, Entry: existing, Entries: List(cfg)}, nil
		}
	}

	return Result{Entry: entry, Entries: List(cfg)}, nil
}

func List(cfg *Config) []Entry {
	if cfg == nil || len(cfg.Static) == 0 {
		return nil
	}

	entries := append([]Entry(nil), cfg.Static...)
	sort.Slice(entries, func(i, j int) bool {
		ai, aj := entries[i].Prefix.Addr(), entries[j].Prefix.Addr()
		if ai.Is4() != aj.Is4() {
			return ai.Is4()
		}
		if cmp := ai.Compare(aj); cmp != 0 {
			return cmp < 0
		}
		return entries[i].Prefix.Bits() < entries[j].Prefix.Bits()
	})
	return entries
}

func ParseEntry(value, comment string) (Entry, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return Entry{}, errors.New("whitelist entry is required")
	}
	if strings.ContainsAny(value, " \t\r\n") {
		return Entry{}, fmt.Errorf("invalid whitelist entry %q", value)
	}
	comment = strings.TrimSpace(comment)
	if strings.IndexFunc(comment, unicode.IsControl) >= 0 {
		return Entry{}, errors.New("comment must not contain control characters")
	}

	if strings.Contains(value, "/") {
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			return Entry{}, fmt.Errorf("invalid IP prefix %q: hostnames are not allowed in the static SSH whitelist", value)
		}
		return Entry{Prefix: prefix.Masked(), Comment: comment}, nil
	}

	addr, err := netip.ParseAddr(value)
	if err != nil {
		return Entry{}, fmt.Errorf("invalid IP address %q: hostnames belong in DDNS whitelist commands", value)
	}
	return Entry{Prefix: netip.PrefixFrom(addr, addr.BitLen()), Comment: comment}, nil
}

func broadWarnings(entry Entry) []Warning {
	bits := entry.Prefix.Bits()
	addr := entry.Prefix.Addr()
	if (addr.Is4() && bits <= 24) || (addr.Is6() && bits <= 64) {
		return []Warning{{
			Code:    WarningBroadPrefix,
			Message: fmt.Sprintf("%s is a broad SSH source prefix; add only addresses or prefixes you control.", entry.Prefix),
		}}
	}
	return nil
}
