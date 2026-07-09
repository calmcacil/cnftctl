package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/calmcacil/cnftctl/internal/apply"
	"github.com/calmcacil/cnftctl/internal/config"
	"github.com/calmcacil/cnftctl/internal/ddns"
	"github.com/calmcacil/cnftctl/internal/features"
	"github.com/calmcacil/cnftctl/internal/install"
	"github.com/calmcacil/cnftctl/internal/nft"
	"github.com/calmcacil/cnftctl/internal/ports"
	"github.com/calmcacil/cnftctl/internal/preset"
	"github.com/calmcacil/cnftctl/internal/render"
	"github.com/calmcacil/cnftctl/internal/systemd"
	"github.com/calmcacil/cnftctl/internal/whitelist"
)

const defaultConfigPath = "/etc/cnftctl/config.yaml"

type realService struct {
	runner nft.Runner
}

func NewService() Service {
	return realService{runner: nft.ExecRunner{}}
}

func (s realService) Run(ctx context.Context, io IO, req CommandRequest) error {
	switch req.Command {
	case "status":
		return s.status(ctx, io, req)
	case "config show":
		return s.configShow(io, req)
	case "init":
		return s.init(io, req)
	case "validate":
		return s.validate(ctx, io, req)
	case "plan":
		return s.plan(io, req)
	case "apply":
		return s.apply(ctx, io, req)
	case "confirm":
		return s.confirm(ctx, io, req)
	case "rollback":
		return s.rollback(ctx, req)
	case "open":
		return s.open(io, req)
	case "close":
		return s.close(io, req)
	case "ports list":
		return s.portsList(io, req)
	case "whitelist add":
		return s.whitelistAdd(io, req)
	case "whitelist remove":
		return s.whitelistRemove(io, req)
	case "whitelist list":
		return s.whitelistList(io, req)
	case "ddns enable":
		return s.ddnsEnable(io, req)
	case "ddns disable":
		return s.ddnsDisable(io, req)
	case "ddns add":
		return s.ddnsAdd(io, req)
	case "ddns remove":
		return s.ddnsRemove(io, req)
	case "ddns set-ipv6-prefix-len":
		return s.ddnsPrefix(io, req)
	case "ddns refresh":
		return s.ddnsRefresh(ctx, io, req)
	case "ddns status":
		return s.ddnsStatus(io, req)
	case "ssh-harden open":
		return s.sshMode(io, req, "open", false)
	case "ssh-harden whitelist-only":
		return s.sshMode(io, req, "whitelist-only", req.BoolFlag("force"))
	case "ssh-harden whitelist-rate-limit":
		return s.sshMode(io, req, "whitelist-rate-limit", req.BoolFlag("force"))
	case "feature enable":
		return s.feature(io, req, true)
	case "feature disable":
		return s.feature(io, req, false)
	case "preset decode":
		return s.presetDecode(io, req)
	case "preset validate":
		return s.presetValidate(io, req)
	case "preset explain":
		return s.presetExplain(io, req)
	default:
		return fmt.Errorf("command %q is not implemented", req.Command)
	}
}

func (s realService) status(ctx context.Context, io IO, req CommandRequest) error {
	root := req.Flag("root")
	st, err := install.InspectStatus(ctx, install.Options{Root: root, Runner: s.runner})
	if err != nil {
		fmt.Fprintf(io.Stdout, "status warning: %v\n", err)
	}
	fmt.Fprintf(io.Stdout, "config: %t\n", st.ConfigExists)
	fmt.Fprintf(io.Stdout, "nftables.conf: %t\n", st.NftablesExists)
	fmt.Fprintf(io.Stdout, "managed table inet hostfw: %t\n", st.TablePresent)
	if st.ValidationError != "" {
		fmt.Fprintf(io.Stdout, "nft validation: failed: %s\n", st.ValidationError)
	} else {
		fmt.Fprintln(io.Stdout, "nft validation: ok")
	}
	cfg, err := loadConfig(req)
	if err == nil {
		fmt.Fprintf(io.Stdout, "docker integration: %t\n", cfg.Docker.Enabled)
		fmt.Fprintf(io.Stdout, "trusted interfaces: %v\n", cfg.TrustedInterfaces.Interfaces)
		fmt.Fprintf(io.Stdout, "DDNS whitelist: enabled=%t hosts=%d\n", cfg.SSH.DDNSWhitelist.Enabled, len(cfg.SSH.DDNSWhitelist.Hosts))
	}
	return nil
}

func (s realService) configShow(io IO, req CommandRequest) error {
	cfg, err := loadConfig(req)
	if err != nil {
		return err
	}
	return config.Save(io.Stdout, cfg)
}

func (s realService) init(io IO, req CommandRequest) error {
	cfg := config.Default()
	if presetValue := req.Flag("preset"); presetValue != "" {
		p, err := preset.DecodeString(presetValue)
		if err != nil {
			return err
		}
		cfg = p.Config
	} else if presetFile := req.Flag("preset-file"); presetFile != "" {
		p, err := readPresetFile(presetFile)
		if err != nil {
			return err
		}
		cfg = p.Config
	}
	if wan := req.Flag("wan-interface"); wan != "" {
		cfg.WANInterface = wan
	}
	if req.BoolFlag("enable-docker") {
		cfg.Docker.Enabled = true
	}
	for _, iface := range req.FlagValues("trust-interface") {
		cfg.TrustedInterfaces.Enabled = true
		cfg.TrustedInterfaces.Interfaces = appendUnique(cfg.TrustedInterfaces.Interfaces, iface)
	}
	if req.BoolFlag("enable-ddns-whitelist") {
		cfg.SSH.DDNSWhitelist.Enabled = true
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	files, err := configAndRenderFiles(req, cfg)
	if err != nil {
		return err
	}
	if req.BoolFlag("dry-run") {
		printFiles(io, files)
		return nil
	}
	if err := writeConfig(req, cfg); err != nil {
		return err
	}
	if err := writeRendered(req, files[1:]); err != nil {
		return err
	}
	fmt.Fprintln(io.Stdout, "initialized cnftctl config and rendered files; run cnftctl apply to load active policy")
	return nil
}

func (s realService) validate(ctx context.Context, io IO, req CommandRequest) error {
	cfg, err := loadConfig(req)
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	files, err := renderApplyFiles(cfg)
	if err != nil {
		return err
	}
	root := firstNonEmpty(req.Flag("output-root"), req.Flag("root"))
	if root != "" {
		if err := writeApplyFiles(root, files); err != nil {
			return err
		}
		if err := nft.ValidateFile(ctx, s.runner, rooted(root, render.NftablesConfPath)); err != nil {
			return err
		}
	} else {
		if err := nft.ValidateFile(ctx, s.runner, render.NftablesConfPath); err != nil {
			return err
		}
	}
	fmt.Fprintln(io.Stdout, "config and rendered nftables are valid")
	return nil
}

func (s realService) plan(io IO, req CommandRequest) error {
	cfg, err := loadConfig(req)
	if err != nil {
		return err
	}
	files, err := renderApplyFiles(cfg)
	if err != nil {
		return err
	}
	plan, err := apply.PlanFiles(firstNonEmpty(req.Flag("output-root"), req.Flag("root")), files, render.NftablesConfPath)
	if err != nil {
		return err
	}
	if len(plan.Changes) == 0 {
		fmt.Fprintln(io.Stdout, "no file changes")
	} else {
		for _, ch := range plan.Changes {
			fmt.Fprintf(io.Stdout, "%s %s\n", ch.Action, ch.Path)
		}
	}
	fmt.Fprintf(io.Stdout, "active nftables would change: %t\n", plan.WouldLoadNftables)
	return nil
}

func (s realService) apply(ctx context.Context, io IO, req CommandRequest) error {
	cfg, err := loadConfig(req)
	if err != nil {
		return err
	}
	files, err := renderApplyFiles(cfg)
	if err != nil {
		return err
	}
	timeout, err := req.DurationFlag("rollback-timeout")
	if err != nil {
		return err
	}
	tx, plan, err := apply.Apply(ctx, apply.Options{
		Root:            req.Flag("root"),
		Files:           files,
		NftConfigPath:   render.NftablesConfPath,
		DryRun:          req.BoolFlag("dry-run"),
		RequireRoot:     !req.BoolFlag("dry-run") && req.Flag("root") == "",
		RollbackTimeout: timeout,
		Runner:          s.runner,
		Systemd:         systemd.Manager{Runner: s.runner},
	})
	if err != nil {
		return err
	}
	if req.BoolFlag("dry-run") {
		for _, ch := range plan.Changes {
			fmt.Fprintf(io.Stdout, "%s %s\n", ch.Action, ch.Path)
		}
		fmt.Fprintln(io.Stdout, "dry-run: no files written and nftables not loaded")
		return nil
	}
	fmt.Fprintf(io.Stdout, "applied transaction %s\n", tx.ID)
	fmt.Fprintf(io.Stdout, "run cnftctl confirm within 120s or rollback will restore previous managed files/rules\n")
	return nil
}

func (s realService) confirm(ctx context.Context, io IO, req CommandRequest) error {
	tx, err := apply.Confirm(ctx, req.Flag("root"), "", "", systemd.Manager{Runner: s.runner})
	if err != nil {
		return err
	}
	fmt.Fprintf(io.Stdout, "confirmed transaction %s\n", tx.ID)
	return nil
}

func (s realService) rollback(ctx context.Context, req CommandRequest) error {
	txDir := req.Flag("transaction-dir")
	if txDir == "" {
		return errors.New("--transaction-dir is required")
	}
	return apply.Restore(ctx, req.Flag("root"), txDir, s.runner)
}

func (s realService) open(io IO, req CommandRequest) error {
	if len(req.Args) != 2 {
		return errors.New("usage: cnftctl open <tcp|udp> <port-or-range>")
	}
	cfg, err := loadConfig(req)
	if err != nil {
		return err
	}
	pc := portsConfig(cfg)
	res, err := ports.Open(&pc, req.Args[0], req.Args[1], req.Flag("comment"))
	if err != nil {
		return err
	}
	setPortsConfig(&cfg, pc)
	if err := saveConfigAndRendered(req, cfg); err != nil {
		return err
	}
	fmt.Fprintf(io.Stdout, "open %s %s changed=%t\n", res.Entry.Protocol, ports.FormatPort(res.Entry), res.Changed)
	printPortWarnings(io, res.Warnings)
	fmt.Fprintln(io.Stdout, "run cnftctl apply to load active policy")
	return nil
}

func (s realService) close(io IO, req CommandRequest) error {
	if len(req.Args) != 2 {
		return errors.New("usage: cnftctl close <tcp|udp> <port-or-range>")
	}
	cfg, err := loadConfig(req)
	if err != nil {
		return err
	}
	pc := portsConfig(cfg)
	res, err := ports.Close(&pc, req.Args[0], req.Args[1], req.BoolFlag("strict"))
	if err != nil {
		return err
	}
	setPortsConfig(&cfg, pc)
	if err := saveConfigAndRendered(req, cfg); err != nil {
		return err
	}
	fmt.Fprintf(io.Stdout, "close %s %s changed=%t\n", res.Entry.Protocol, ports.FormatPort(res.Entry), res.Changed)
	fmt.Fprintln(io.Stdout, "run cnftctl apply to load active policy")
	return nil
}

func (s realService) portsList(io IO, req CommandRequest) error {
	cfg, err := loadConfig(req)
	if err != nil {
		return err
	}
	for _, entry := range ports.List(&ports.Config{OpenPorts: portEntries(cfg.OpenPorts)}) {
		line := fmt.Sprintf("%s %s", entry.Protocol, ports.FormatPort(entry))
		if entry.Comment != "" {
			line += " # " + entry.Comment
		}
		fmt.Fprintln(io.Stdout, line)
	}
	fmt.Fprintln(io.Stdout, "configured ports may differ from active policy until cnftctl apply is confirmed")
	return nil
}

func (s realService) whitelistAdd(io IO, req CommandRequest) error {
	if len(req.Args) != 1 {
		return errors.New("usage: cnftctl whitelist add <ip-or-cidr>")
	}
	cfg, err := loadConfig(req)
	if err != nil {
		return err
	}
	wc := whitelistConfig(cfg)
	res, err := whitelist.Add(&wc, req.Args[0], req.Flag("comment"))
	if err != nil {
		return err
	}
	setWhitelistConfig(&cfg, wc)
	if err := saveConfigAndRendered(req, cfg); err != nil {
		return err
	}
	fmt.Fprintf(io.Stdout, "whitelist add %s changed=%t\n", res.Entry.Prefix, res.Changed)
	for _, w := range res.Warnings {
		fmt.Fprintf(io.Stderr, "warning: %s\n", w.Message)
	}
	fmt.Fprintln(io.Stdout, "run cnftctl apply to load active policy")
	return nil
}

func (s realService) whitelistRemove(io IO, req CommandRequest) error {
	if len(req.Args) != 1 {
		return errors.New("usage: cnftctl whitelist remove <ip-or-cidr>")
	}
	cfg, err := loadConfig(req)
	if err != nil {
		return err
	}
	wc := whitelistConfig(cfg)
	res, err := whitelist.Remove(&wc, req.Args[0])
	if err != nil {
		return err
	}
	if req.BoolFlag("strict") && !res.Changed {
		return fmt.Errorf("%s is not in the static SSH whitelist", req.Args[0])
	}
	setWhitelistConfig(&cfg, wc)
	if err := saveConfigAndRendered(req, cfg); err != nil {
		return err
	}
	fmt.Fprintf(io.Stdout, "whitelist remove %s changed=%t\n", res.Entry.Prefix, res.Changed)
	fmt.Fprintln(io.Stdout, "run cnftctl apply to load active policy")
	return nil
}

func (s realService) whitelistList(io IO, req CommandRequest) error {
	cfg, err := loadConfig(req)
	if err != nil {
		return err
	}
	for _, entry := range whitelist.List(&whitelist.Config{Static: whitelistEntries(cfg)}) {
		fmt.Fprintln(io.Stdout, entry.Prefix)
	}
	return nil
}

func (s realService) ddnsEnable(io IO, req CommandRequest) error {
	cfg, err := loadConfig(req)
	if err != nil {
		return err
	}
	dc := ddnsConfig(cfg)
	changed, err := ddns.Enable(&dc)
	if err != nil {
		return err
	}
	setDDNSConfig(&cfg, dc)
	if err := saveConfigAndRendered(req, cfg); err != nil {
		return err
	}
	fmt.Fprintf(io.Stdout, "DDNS whitelist enabled changed=%t\n", changed)
	fmt.Fprintln(io.Stdout, "run cnftctl apply to load active policy")
	return nil
}

func (s realService) ddnsDisable(io IO, req CommandRequest) error {
	cfg, err := loadConfig(req)
	if err != nil {
		return err
	}
	dc := ddnsConfig(cfg)
	changed, err := ddns.Disable(&dc)
	if err != nil {
		return err
	}
	setDDNSConfig(&cfg, dc)
	if err := saveConfigAndRendered(req, cfg); err != nil {
		return err
	}
	fmt.Fprintf(io.Stdout, "DDNS whitelist disabled changed=%t\n", changed)
	fmt.Fprintln(io.Stdout, "run cnftctl apply to load active policy")
	return nil
}

func (s realService) ddnsAdd(io IO, req CommandRequest) error {
	if len(req.Args) != 1 {
		return errors.New("usage: cnftctl ddns add <hostname>")
	}
	cfg, err := loadConfig(req)
	if err != nil {
		return err
	}
	dc := ddnsConfig(cfg)
	changed, err := ddns.AddHost(&dc, req.Args[0])
	if err != nil {
		return err
	}
	setDDNSConfig(&cfg, dc)
	if err := saveConfigAndRendered(req, cfg); err != nil {
		return err
	}
	fmt.Fprintf(io.Stdout, "DDNS host add changed=%t\n", changed)
	fmt.Fprintln(io.Stdout, "run cnftctl apply to load active policy")
	return nil
}

func (s realService) ddnsRemove(io IO, req CommandRequest) error {
	if len(req.Args) != 1 {
		return errors.New("usage: cnftctl ddns remove <hostname>")
	}
	cfg, err := loadConfig(req)
	if err != nil {
		return err
	}
	dc := ddnsConfig(cfg)
	changed, err := ddns.RemoveHost(&dc, req.Args[0])
	if err != nil {
		return err
	}
	if req.BoolFlag("strict") && !changed {
		return fmt.Errorf("DDNS host %s is not configured", req.Args[0])
	}
	setDDNSConfig(&cfg, dc)
	if err := saveConfigAndRendered(req, cfg); err != nil {
		return err
	}
	fmt.Fprintf(io.Stdout, "DDNS host remove changed=%t\n", changed)
	fmt.Fprintln(io.Stdout, "run cnftctl apply to load active policy")
	return nil
}

func (s realService) ddnsPrefix(io IO, req CommandRequest) error {
	if len(req.Args) != 1 {
		return errors.New("usage: cnftctl ddns set-ipv6-prefix-len <56|64>")
	}
	prefixLen, err := strconv.Atoi(req.Args[0])
	if err != nil {
		return err
	}
	cfg, err := loadConfig(req)
	if err != nil {
		return err
	}
	dc := ddnsConfig(cfg)
	changed, err := ddns.SetIPv6PrefixLen(&dc, prefixLen)
	if err != nil {
		return err
	}
	setDDNSConfig(&cfg, dc)
	if err := saveConfigAndRendered(req, cfg); err != nil {
		return err
	}
	fmt.Fprintf(io.Stdout, "DDNS IPv6 prefix length set to /%d changed=%t\n", prefixLen, changed)
	fmt.Fprintln(io.Stdout, "run cnftctl apply to load active policy")
	return nil
}

func (s realService) ddnsRefresh(ctx context.Context, io IO, req CommandRequest) error {
	cfg, err := loadConfig(req)
	if err != nil {
		return err
	}
	result, err := ddns.Refresh(ctx, ddnsConfig(cfg), ddns.NetResolver{}, nftRuntime{runner: s.runner})
	if err != nil {
		return err
	}
	fmt.Fprintf(io.Stdout, "refreshed DDNS whitelist: %d IPv4 entries, %d IPv6 prefixes\n", len(result.IPv4), len(result.IPv6))
	return nil
}

func (s realService) ddnsStatus(io IO, req CommandRequest) error {
	cfg, err := loadConfig(req)
	if err != nil {
		return err
	}
	fmt.Fprintf(io.Stdout, "enabled: %t\n", cfg.SSH.DDNSWhitelist.Enabled)
	fmt.Fprintf(io.Stdout, "ipv6_prefix_len: %d\n", cfg.SSH.DDNSWhitelist.IPv6PrefixLen)
	for _, host := range cfg.SSH.DDNSWhitelist.Hosts {
		fmt.Fprintf(io.Stdout, "host: %s\n", host)
	}
	return nil
}

func (s realService) sshMode(io IO, req CommandRequest, mode string, force bool) error {
	cfg, err := loadConfig(req)
	if err != nil {
		return err
	}
	fc := featuresConfig(cfg)
	res, err := features.SetSSHMode(&fc, mode, force)
	if err != nil {
		return err
	}
	cfg.SSH.Mode = fc.SSH.Mode
	if mode == "whitelist-rate-limit" && cfg.SSH.RateLimit == nil {
		cfg.SSH.RateLimit = &config.RateLimit{Connections: 6, Per: config.Duration{Duration: time.Minute}}
	}
	if err := saveConfigAndRendered(req, cfg); err != nil {
		return err
	}
	fmt.Fprintf(io.Stdout, "SSH mode set to %s changed=%t\n", mode, res.Changed)
	for _, w := range res.Warnings {
		fmt.Fprintf(io.Stderr, "warning: %s\n", w.Message)
	}
	fmt.Fprintln(io.Stdout, "run cnftctl apply to load active policy")
	return nil
}

func (s realService) feature(io IO, req CommandRequest, enable bool) error {
	if len(req.Args) != 1 {
		return errors.New("usage: cnftctl feature <enable|disable> <docker|trusted-interface>")
	}
	cfg, err := loadConfig(req)
	if err != nil {
		return err
	}
	fc := featuresConfig(cfg)
	var res features.Result
	switch req.Args[0] {
	case "docker":
		if enable {
			res, err = features.EnableDocker(&fc)
		} else {
			res, err = features.DisableDocker(&fc)
		}
		cfg.Docker.Enabled = fc.Docker.Enabled
	case "trusted-interface":
		ifaces := req.FlagValues("interface")
		if len(ifaces) == 0 {
			return errors.New("--interface is required for trusted-interface")
		}
		for _, iface := range ifaces {
			if enable {
				res, err = features.EnableTrustedInterface(&fc, iface)
			} else {
				res, err = features.DisableTrustedInterface(&fc, iface)
			}
			if err != nil {
				return err
			}
		}
		cfg.TrustedInterfaces.Enabled = fc.TrustedInterfaces.Enabled
		cfg.TrustedInterfaces.Interfaces = fc.TrustedInterfaces.Interfaces
	default:
		return fmt.Errorf("unknown feature %q", req.Args[0])
	}
	if err != nil {
		return err
	}
	if err := saveConfigAndRendered(req, cfg); err != nil {
		return err
	}
	fmt.Fprintf(io.Stdout, "feature %s changed=%t\n", req.Args[0], res.Changed)
	for _, w := range res.Warnings {
		fmt.Fprintf(io.Stderr, "warning: %s\n", w.Message)
	}
	fmt.Fprintln(io.Stdout, "run cnftctl apply to load active policy")
	return nil
}

func (s realService) presetDecode(io IO, req CommandRequest) error {
	if len(req.Args) != 1 {
		return errors.New("usage: cnftctl preset decode <preset>")
	}
	p, err := preset.DecodeString(req.Args[0])
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(p.Config, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(io.Stdout, string(data))
	return nil
}

func (s realService) presetValidate(io IO, req CommandRequest) error {
	if len(req.Args) != 1 {
		return errors.New("usage: cnftctl preset validate <file>")
	}
	p, err := readPresetFile(req.Args[0])
	if err != nil {
		return err
	}
	if err := p.Validate(); err != nil {
		return err
	}
	fmt.Fprintln(io.Stdout, "preset is valid")
	return nil
}

func (s realService) presetExplain(io IO, req CommandRequest) error {
	if len(req.Args) != 1 {
		return errors.New("usage: cnftctl preset explain <file>")
	}
	p, err := readPresetFile(req.Args[0])
	if err != nil {
		return err
	}
	for _, line := range p.Explain() {
		fmt.Fprintln(io.Stdout, line)
	}
	return nil
}

func loadConfig(req CommandRequest) (config.Config, error) {
	path := rooted(req.Flag("root"), configPath(req))
	cfg, err := config.LoadFile(path)
	if err != nil {
		return config.Config{}, fmt.Errorf("load config %s: %w", path, err)
	}
	return cfg, nil
}

func writeConfig(req CommandRequest, cfg config.Config) error {
	path := rooted(req.Flag("root"), configPath(req))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return config.SaveFile(path, cfg, 0o600)
}

func configPath(req CommandRequest) string {
	if p := req.Flag("config"); p != "" {
		return p
	}
	return defaultConfigPath
}

func saveConfigAndRendered(req CommandRequest, cfg config.Config) error {
	if err := writeConfig(req, cfg); err != nil {
		return err
	}
	files, err := renderApplyFiles(cfg)
	if err != nil {
		return err
	}
	return writeRendered(req, files)
}

func configAndRenderFiles(req CommandRequest, cfg config.Config) ([]apply.File, error) {
	files, err := renderApplyFiles(cfg)
	if err != nil {
		return nil, err
	}
	var cfgBuf bytes.Buffer
	if err := config.Save(&cfgBuf, cfg); err != nil {
		return nil, err
	}
	return append([]apply.File{{Path: configPath(req), Mode: 0o600, Data: cfgBuf.Bytes()}}, files...), nil
}

func writeRendered(req CommandRequest, files []apply.File) error {
	return writeApplyFiles(req.Flag("root"), files)
}

func writeApplyFiles(root string, files []apply.File) error {
	for _, file := range files {
		path := rooted(root, file.Path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		mode := file.Mode
		if mode == 0 {
			mode = 0o644
		}
		if err := os.WriteFile(path, file.Data, mode); err != nil {
			return err
		}
	}
	return nil
}

func renderApplyFiles(cfg config.Config) ([]apply.File, error) {
	rendered, err := render.Files(renderConfig(cfg))
	if err != nil {
		return nil, err
	}
	files := make([]apply.File, 0, len(rendered))
	for _, file := range rendered {
		files = append(files, apply.File{Path: file.Path, Mode: 0o644, Data: []byte(file.Content)})
	}
	return files, nil
}

func renderConfig(cfg config.Config) render.Config {
	openPorts := make([]render.OpenPort, 0, len(cfg.OpenPorts))
	for _, p := range cfg.OpenPorts {
		port := strconv.Itoa(p.Port)
		if p.EndPort != 0 && p.EndPort != p.Port {
			port = fmt.Sprintf("%d-%d", p.Port, p.EndPort)
		}
		openPorts = append(openPorts, render.OpenPort{Protocol: p.Protocol, Port: port, Comment: p.Comment})
	}
	rateLimit := ""
	if cfg.SSH.RateLimit != nil {
		per := "second"
		if cfg.SSH.RateLimit.Per.Duration >= time.Hour {
			per = "hour"
		} else if cfg.SSH.RateLimit.Per.Duration >= time.Minute {
			per = "minute"
		}
		rateLimit = fmt.Sprintf("%d/%s burst 3 packets", cfg.SSH.RateLimit.Connections, per)
	}
	return render.Config{
		WANInterface: cfg.WANInterface,
		OpenPorts:    openPorts,
		SSH: render.SSHConfig{
			Mode:      render.SSHMode(cfg.SSH.Mode),
			RateLimit: rateLimit,
			StaticWhitelist: render.StaticWhitelist{
				IPv4: cfg.SSH.StaticWhitelist.IPv4,
				IPv6: cfg.SSH.StaticWhitelist.IPv6,
			},
			DDNSWhitelist: render.DDNSWhitelist{
				Enabled:         cfg.SSH.DDNSWhitelist.Enabled,
				Hosts:           cfg.SSH.DDNSWhitelist.Hosts,
				TTL:             cfg.SSH.DDNSWhitelist.TTL.Duration,
				RefreshInterval: cfg.SSH.DDNSWhitelist.RefreshInterval.Duration,
				IPv6PrefixLen:   cfg.SSH.DDNSWhitelist.IPv6PrefixLen,
			},
		},
		TrustedInterfaces: render.TrustedInterfacesConfig{
			Enabled:         cfg.TrustedInterfaces.Enabled,
			Interfaces:      cfg.TrustedInterfaces.Interfaces,
			TrustForwarding: cfg.TrustedInterfaces.TrustForwarding,
		},
		Docker: render.DockerConfig{Enabled: cfg.Docker.Enabled, Interfaces: cfg.Docker.Interfaces},
	}
}

func portsConfig(cfg config.Config) ports.Config {
	return ports.Config{OpenPorts: portEntries(cfg.OpenPorts), DockerEnabled: cfg.Docker.Enabled}
}

func portEntries(values []config.OpenPort) []ports.Entry {
	out := make([]ports.Entry, 0, len(values))
	for _, p := range values {
		end := p.EndPort
		if end == 0 {
			end = p.Port
		}
		out = append(out, ports.Entry{Protocol: p.Protocol, Start: uint16(p.Port), End: uint16(end), Comment: p.Comment})
	}
	return out
}

func setPortsConfig(cfg *config.Config, pc ports.Config) {
	cfg.OpenPorts = cfg.OpenPorts[:0]
	for _, p := range ports.List(&pc) {
		entry := config.OpenPort{Protocol: p.Protocol, Port: int(p.Start), Comment: p.Comment}
		if p.End != p.Start {
			entry.EndPort = int(p.End)
		}
		cfg.OpenPorts = append(cfg.OpenPorts, entry)
	}
}

func whitelistConfig(cfg config.Config) whitelist.Config {
	return whitelist.Config{Static: whitelistEntries(cfg)}
}

func whitelistEntries(cfg config.Config) []whitelist.Entry {
	var entries []whitelist.Entry
	for _, value := range append(append([]string{}, cfg.SSH.StaticWhitelist.IPv4...), cfg.SSH.StaticWhitelist.IPv6...) {
		entry, err := whitelist.ParseEntry(value, "")
		if err == nil {
			entries = append(entries, entry)
		}
	}
	return entries
}

func setWhitelistConfig(cfg *config.Config, wc whitelist.Config) {
	cfg.SSH.StaticWhitelist.IPv4 = nil
	cfg.SSH.StaticWhitelist.IPv6 = nil
	for _, entry := range whitelist.List(&wc) {
		text := prefixText(entry.Prefix)
		if entry.Prefix.Addr().Is4() {
			cfg.SSH.StaticWhitelist.IPv4 = append(cfg.SSH.StaticWhitelist.IPv4, text)
		} else {
			cfg.SSH.StaticWhitelist.IPv6 = append(cfg.SSH.StaticWhitelist.IPv6, text)
		}
	}
}

func ddnsConfig(cfg config.Config) ddns.Config {
	return ddns.Config{
		Enabled:       cfg.SSH.DDNSWhitelist.Enabled,
		Hosts:         cfg.SSH.DDNSWhitelist.Hosts,
		IPv6PrefixLen: cfg.SSH.DDNSWhitelist.IPv6PrefixLen,
		TTL:           cfg.SSH.DDNSWhitelist.TTL.Duration,
	}
}

func setDDNSConfig(cfg *config.Config, dc ddns.Config) {
	cfg.SSH.DDNSWhitelist.Enabled = dc.Enabled
	cfg.SSH.DDNSWhitelist.Hosts = dc.Hosts
	cfg.SSH.DDNSWhitelist.IPv6PrefixLen = dc.IPv6PrefixLen
	cfg.SSH.DDNSWhitelist.TTL = config.Duration{Duration: dc.TTL}
}

func featuresConfig(cfg config.Config) features.Config {
	return features.Config{
		SSH: features.SSHConfig{
			Mode:            cfg.SSH.Mode,
			StaticWhitelist: append(append([]string{}, cfg.SSH.StaticWhitelist.IPv4...), cfg.SSH.StaticWhitelist.IPv6...),
			DDNSEnabled:     cfg.SSH.DDNSWhitelist.Enabled,
			DDNSHosts:       cfg.SSH.DDNSWhitelist.Hosts,
		},
		TrustedInterfaces: features.TrustedInterfacesConfig{Enabled: cfg.TrustedInterfaces.Enabled, Interfaces: append([]string{}, cfg.TrustedInterfaces.Interfaces...)},
		Docker:            features.DockerConfig{Enabled: cfg.Docker.Enabled},
	}
}

type nftRuntime struct {
	runner nft.Runner
}

func (r nftRuntime) Refresh(ctx context.Context, ipv4 []netip.Addr, ipv6 []netip.Prefix, ttl time.Duration) error {
	args := []string{"flush", "set", "inet", "hostfw", "ddns_whitelist_v4"}
	if res := r.runner.Run(ctx, "nft", args...); !res.OK() {
		return res.Error()
	}
	args = []string{"flush", "set", "inet", "hostfw", "ddns_whitelist_v6"}
	if res := r.runner.Run(ctx, "nft", args...); !res.OK() {
		return res.Error()
	}
	for _, addr := range ipv4 {
		if res := r.runner.Run(ctx, "nft", "add", "element", "inet", "hostfw", "ddns_whitelist_v4", "{", addr.String(), "timeout", nftTimeout(ttl), "}"); !res.OK() {
			return res.Error()
		}
	}
	for _, prefix := range ipv6 {
		if res := r.runner.Run(ctx, "nft", "add", "element", "inet", "hostfw", "ddns_whitelist_v6", "{", prefix.String(), "timeout", nftTimeout(ttl), "}"); !res.OK() {
			return res.Error()
		}
	}
	return nil
}

func (r nftRuntime) List(context.Context) (ddns.RuntimeEntries, error) {
	return ddns.RuntimeEntries{}, nil
}

func readPresetFile(path string) (preset.Preset, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return preset.Preset{}, err
	}
	return preset.DecodeJSON(data)
}

func printFiles(io IO, files []apply.File) {
	for _, file := range files {
		fmt.Fprintf(io.Stdout, "--- %s ---\n%s", file.Path, file.Data)
		if len(file.Data) == 0 || file.Data[len(file.Data)-1] != '\n' {
			fmt.Fprintln(io.Stdout)
		}
	}
}

func printPortWarnings(io IO, warnings []ports.Warning) {
	for _, w := range warnings {
		fmt.Fprintf(io.Stderr, "warning: %s\n", w.Message)
	}
}

func prefixText(prefix netip.Prefix) string {
	if prefix.Bits() == prefix.Addr().BitLen() {
		return prefix.Addr().String()
	}
	return prefix.String()
}

func rooted(root, path string) string {
	if root == "" {
		return path
	}
	return filepath.Join(root, strings.TrimPrefix(filepath.Clean(path), string(filepath.Separator)))
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func nftTimeout(d time.Duration) string {
	if d <= 0 {
		d = time.Hour
	}
	if d%time.Hour == 0 {
		return fmt.Sprintf("%dh", int(d/time.Hour))
	}
	if d%time.Minute == 0 {
		return fmt.Sprintf("%dm", int(d/time.Minute))
	}
	return fmt.Sprintf("%ds", int(d/time.Second))
}
