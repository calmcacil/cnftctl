package install

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/calmcacil/cnftctl/internal/apply"
	"github.com/calmcacil/cnftctl/internal/nft"
)

const (
	NftablesConfPath       = "/etc/nftables.conf"
	ReferenceNftablesDir   = "/etc/nftables.d"
	ReferenceOpenPortsPath = "/etc/nftables.d/open-ports.nft"
	ReferenceWhitelistPath = "/etc/nftables.d/whitelist.nft"
	ManagedOpenPortsPath   = "/etc/cnftctl/nftables.d/open-ports.nft"
	ManagedWhitelistPath   = "/etc/cnftctl/nftables.d/whitelist.nft"
	ConfigPath             = "/etc/cnftctl/config.yaml"
)

type PrivilegeChecker func() int

type Options struct {
	Root        string
	Runner      nft.Runner
	EUID        PrivilegeChecker
	RequireRoot bool
	Files       []apply.File
}

type Status struct {
	ConfigExists    bool
	NftablesExists  bool
	OpenPortsExists bool
	WhitelistExists bool
	TablePresent    bool
	ValidationError string
}

type Adoption struct {
	OpenPorts []Port
	Whitelist Whitelist
	Warnings  []string
}

type Port struct {
	Protocol string
	Port     string
}

type Whitelist struct {
	IPv4 []string
	IPv6 []string
}

func CheckRoot(euid PrivilegeChecker) error {
	if euid == nil {
		euid = os.Geteuid
	}
	if euid() != 0 {
		return errors.New("root privileges are required")
	}
	return nil
}

func CheckDependencies(ctx context.Context, runner nft.Runner, extra ...string) error {
	if runner == nil {
		runner = nft.ExecRunner{}
	}
	deps := append([]string{"nft", "systemctl", "systemd-run"}, extra...)
	return nft.CheckDependencies(ctx, runner, deps...)
}

func Init(ctx context.Context, opts Options) (apply.Plan, error) {
	if opts.RequireRoot {
		if err := CheckRoot(opts.EUID); err != nil {
			return apply.Plan{}, err
		}
	}
	if err := CheckDependencies(ctx, runner(opts)); err != nil {
		return apply.Plan{}, err
	}
	return apply.PlanFiles(opts.Root, opts.Files, NftablesConfPath)
}

func Validate(ctx context.Context, opts Options) error {
	if opts.RequireRoot {
		if err := CheckRoot(opts.EUID); err != nil {
			return err
		}
	}
	return nft.ValidateFile(ctx, runner(opts), rooted(opts.Root, NftablesConfPath))
}

func Plan(opts Options) (apply.Plan, error) {
	return apply.PlanFiles(opts.Root, opts.Files, NftablesConfPath)
}

func InspectStatus(ctx context.Context, opts Options) (Status, error) {
	var st Status
	st.ConfigExists = exists(rooted(opts.Root, ConfigPath))
	st.NftablesExists = exists(rooted(opts.Root, NftablesConfPath))
	st.OpenPortsExists = exists(rooted(opts.Root, ManagedOpenPortsPath))
	st.WhitelistExists = exists(rooted(opts.Root, ManagedWhitelistPath))
	present, err := nft.HasTable(ctx, runner(opts), "inet", "hostfw")
	if err != nil {
		return st, err
	}
	st.TablePresent = present
	if err := nft.ValidateFile(ctx, runner(opts), rooted(opts.Root, NftablesConfPath)); err != nil {
		st.ValidationError = err.Error()
	}
	return st, nil
}

func AdoptReference(root string) (Adoption, error) {
	var adoption Adoption
	ports, warnings, err := ParseOpenPortsFile(rooted(root, ReferenceOpenPortsPath))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return adoption, err
	}
	adoption.OpenPorts = ports
	adoption.Warnings = append(adoption.Warnings, warnings...)
	whitelist, warnings, err := ParseWhitelistFile(rooted(root, ReferenceWhitelistPath))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return adoption, err
	}
	adoption.Whitelist = whitelist
	adoption.Warnings = append(adoption.Warnings, warnings...)
	return adoption, nil
}

func ReferenceFiles(referenceRoot string) ([]apply.File, error) {
	mapping := map[string]string{
		filepath.Join(referenceRoot, "nftables.conf"):                NftablesConfPath,
		filepath.Join(referenceRoot, "nftables.d", "open-ports.nft"): ReferenceOpenPortsPath,
		filepath.Join(referenceRoot, "nftables.d", "whitelist.nft"):  ReferenceWhitelistPath,
	}
	var files []apply.File
	for src, dst := range mapping {
		data, err := os.ReadFile(src)
		if err != nil {
			return nil, err
		}
		files = append(files, apply.File{Path: dst, Data: data, Mode: 0o644})
	}
	return files, nil
}

var portRE = regexp.MustCompile(`(?i)^\s*(tcp|udp)\s*\.\s*([0-9]+(?:-[0-9]+)?)\s*,?`)

func ParseOpenPortsFile(path string) ([]Port, []string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var ports []Port
	var warnings []string
	seen := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := stripComment(line)
		m := portRE.FindStringSubmatch(trimmed)
		if m == nil {
			if strings.Contains(trimmed, ".") && !strings.Contains(trimmed, "typeof") {
				warnings = append(warnings, fmt.Sprintf("unsupported open port line: %s", strings.TrimSpace(line)))
			}
			continue
		}
		key := strings.ToLower(m[1]) + "/" + m[2]
		if seen[key] {
			continue
		}
		seen[key] = true
		ports = append(ports, Port{Protocol: strings.ToLower(m[1]), Port: m[2]})
	}
	return ports, warnings, nil
}

func ParseWhitelistFile(path string) (Whitelist, []string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Whitelist{}, nil, err
	}
	var w Whitelist
	var warnings []string
	section := ""
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(stripComment(raw))
		switch {
		case strings.HasPrefix(line, "define whitelist_v4"):
			section = "v4"
			line = strings.TrimSpace(strings.TrimPrefix(line, "define whitelist_v4"))
		case strings.HasPrefix(line, "define whitelist_v6"):
			section = "v6"
			line = strings.TrimSpace(strings.TrimPrefix(line, "define whitelist_v6"))
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "="))
		line = strings.Trim(line, "{} ")
		if line == "" {
			continue
		}
		for _, part := range strings.Split(line, ",") {
			entry := strings.TrimSpace(part)
			if entry == "" || entry == "{" || entry == "}" {
				continue
			}
			if strings.Contains(entry, "$") {
				warnings = append(warnings, fmt.Sprintf("unsupported whitelist expression: %s", entry))
				continue
			}
			switch section {
			case "v4":
				w.IPv4 = append(w.IPv4, entry)
			case "v6":
				w.IPv6 = append(w.IPv6, entry)
			default:
				warnings = append(warnings, fmt.Sprintf("whitelist entry outside known define: %s", entry))
			}
		}
	}
	return w, warnings, nil
}

func stripComment(line string) string {
	if i := strings.Index(line, "#"); i >= 0 {
		return line[:i]
	}
	return line
}

func runner(opts Options) nft.Runner {
	if opts.Runner != nil {
		return opts.Runner
	}
	return nft.ExecRunner{}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func rooted(root, path string) string {
	if root == "" {
		return path
	}
	return filepath.Join(root, strings.TrimPrefix(filepath.Clean(path), string(filepath.Separator)))
}
