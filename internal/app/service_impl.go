package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/calmcacil/cnftctl/internal/apply"
	"github.com/calmcacil/cnftctl/internal/config"
	"github.com/calmcacil/cnftctl/internal/ddns"
	"github.com/calmcacil/cnftctl/internal/docker"
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
	runner   nft.Runner
	resolver ddns.Resolver
}

func NewService() Service {
	return realService{runner: nft.ExecRunner{}}
}

func (s realService) Run(ctx context.Context, io IO, req CommandRequest) error {
	if err := validateExecutionMode(req); err != nil {
		return err
	}
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
		return s.rollback(ctx, io, req)
	case "reconcile":
		return s.reconcile(ctx, io, req)
	case "doctor":
		return s.doctor(ctx, io, req)
	case "transactions list":
		return s.transactionsList(io, req)
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
		return s.ddnsStatus(ctx, io, req)
	case "ddns timer status":
		return s.ddnsTimerStatus(ctx, io)
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
	case "docker status":
		return s.dockerStatus(io, req)
	case "docker backend plan":
		return s.dockerBackend(ctx, io, req, false)
	case "docker backend write":
		return s.dockerBackend(ctx, io, req, true)
	case "adopt reference":
		return s.adoptReference(io, req)
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
	configExists := exists(rooted(root, configPath(req)))
	checks := []Check{platformCheckAt(rooted(root, "/etc/os-release")), existenceCheck("install.config", configExists, "desired config")}
	checks = append(checks, inspectDurableState(root)...)
	cfg, cfgErr := loadConfig(req)
	if cfgErr != nil {
		checks = append(checks, Check{ID: "config.valid", State: stateForMissing(configExists), Summary: "desired config unavailable", Code: "config_load_failed", Detail: map[string]any{"error": cfgErr.Error()}}, Check{ID: "desired_active.drift", State: StateUnknown, Summary: "not checked because desired config is unavailable", Code: "config_unavailable"})
	} else {
		checks = append(checks, Check{ID: "config.valid", State: StateOK, Summary: "desired config is valid"})
		if inSync, driftErr := renderedInSync(root, cfg); driftErr != nil {
			checks = append(checks, Check{ID: "desired_active.drift", State: StateUnknown, Summary: "drift could not be determined", Code: "drift_inspection_failed", Detail: map[string]any{"error": driftErr.Error()}})
		} else if inSync {
			checks = append(checks, Check{ID: "desired_active.drift", State: StateOK, Summary: "desired and active generation agree"})
		} else {
			checks = append(checks, Check{ID: "desired_active.drift", State: StateDegraded, Summary: "desired policy differs from active generation", Code: "desired_active_drift"})
		}
		if root == "" {
			checks = append(checks, ddnsChecks(ctx, cfg, s.runner)...)
			checks = append(checks, ddnsTimerChecks(ctx, cfg, s.runner)...)
		}
	}
	if root == "" {
		checks = append(checks, liveFirewallChecks(ctx, s.runner)...)
		checks = append(checks, pendingChecks(ctx, root, s.runner)...)
	}
	return finishReport(io, req, newReport(req.Command, checks, nil))
}

func platformCheckAt(path string) Check {
	return platformCheck(path, runtime.GOOS, runtime.GOARCH)
}

func platformCheck(path, goos, goarch string) Check {
	detail := map[string]any{"os": goos, "architecture": goarch}
	if goos == "linux" && (goarch == "amd64" || goarch == "arm64") {
		data, err := os.ReadFile(path)
		if err != nil {
			return Check{ID: "platform.support", State: StateUnknown, Summary: fmt.Sprintf("Linux %s detected; Debian release unknown", goarch), Code: "os_release_unavailable", Detail: detail}
		}
		values, err := parseOSRelease(data)
		if err == nil && values["ID"] == "debian" && values["VERSION_ID"] == "13" {
			if goarch == "amd64" {
				detail["support_tier"] = "production"
				detail["production_validated"] = true
				return Check{ID: "platform.support", State: StateOK, Summary: "Debian 13 amd64 is production-supported", Detail: detail}
			}
			detail["support_tier"] = "experimental"
			detail["production_validated"] = false
			detail["risk"] = "unvalidated on a disposable live host; use at own risk"
			return Check{ID: "platform.support", State: StateOK, Summary: "Debian 13 arm64 is experimental, unvalidated, and used at own risk", Detail: detail}
		}
		return Check{ID: "platform.support", State: StateUnsupported, Summary: "only Debian 13 amd64 and experimental arm64 are available", Code: "unsupported_platform", Detail: detail}
	}
	return Check{ID: "platform.support", State: StateUnsupported, Summary: "only Debian 13 amd64 and experimental arm64 are available", Code: "unsupported_platform", Detail: detail}
}

func parseOSRelease(data []byte) (map[string]string, error) {
	values := map[string]string{}
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || key == "" {
			return nil, fmt.Errorf("invalid os-release line %q", raw)
		}
		if strings.HasPrefix(value, "\"") {
			unquoted, err := strconv.Unquote(value)
			if err != nil {
				return nil, err
			}
			value = unquoted
		} else if strings.ContainsAny(value, " \t\"'") {
			return nil, fmt.Errorf("invalid unquoted os-release value for %s", key)
		}
		values[key] = value
	}
	return values, nil
}

func existenceCheck(id string, exists bool, name string) Check {
	return boolCheck(id, exists, name+" present", StateAbsent, "asset_absent")
}

func boolCheck(id string, value bool, success string, failure State, code string) Check {
	if value {
		return Check{ID: id, State: StateOK, Summary: success}
	}
	return Check{ID: id, State: failure, Summary: strings.TrimSuffix(success, " present") + " absent", Code: code}
}

func stateForMissing(exists bool) State {
	if exists {
		return StateFailed
	}
	return StateAbsent
}

func inspectDurableState(root string) []Check {
	activePath := rooted(root, apply.ActiveSelector)
	target, err := os.Readlink(activePath)
	if errors.Is(err, os.ErrNotExist) {
		return []Check{{ID: "generation.active", State: StateAbsent, Summary: "no active generation", Code: "active_generation_absent"}, {ID: "generation.manifest", State: StateNotApplicable, Summary: "no active generation", Code: "active_generation_absent"}, {ID: "ownership", State: StateAbsent, Summary: "ownership is not established", Code: "ownership_absent"}}
	}
	if err != nil {
		return []Check{{ID: "generation.active", State: StateFailed, Summary: "active generation selector is unreadable", Code: "active_selector_invalid", Detail: map[string]any{"error": err.Error()}}, {ID: "generation.manifest", State: StateUnknown, Summary: "not checked because active selector is invalid", Code: "active_selector_invalid"}, {ID: "ownership", State: StateUnknown, Summary: "not checked because active selector is invalid", Code: "active_selector_invalid"}}
	}
	generation := filepath.Base(target)
	if len(generation) != 64 {
		return []Check{{ID: "generation.active", State: StateFailed, Summary: "active generation selector is invalid", Code: "active_selector_invalid"}}
	}
	checks := []Check{{ID: "generation.active", State: StateOK, Summary: "active generation selected", Detail: map[string]any{"generation": generation}}}
	manifestPath := rooted(root, apply.GenerationRoot+"/"+generation+"/manifest.json")
	manifestData, manifestErr := os.ReadFile(manifestPath)
	var manifest apply.Manifest
	if manifestErr != nil || json.Unmarshal(manifestData, &manifest) != nil || manifest.Version != 1 {
		checks = append(checks, Check{ID: "generation.manifest", State: StateFailed, Summary: "active generation manifest is invalid", Code: "manifest_invalid"})
	} else {
		valid := true
		for _, entry := range manifest.Files {
			data, readErr := os.ReadFile(filepath.Join(filepath.Dir(manifestPath), filepath.Base(entry.Path)))
			sum := sha256.Sum256(data)
			if readErr != nil || fmt.Sprintf("%x", sum) != entry.SHA256 || len(data) != entry.Size {
				valid = false
				break
			}
		}
		checks = append(checks, boolCheck("generation.manifest", valid, "active generation manifest and files are valid", StateFailed, "manifest_invalid"))
	}
	var owner struct{ Marker, Generation string }
	ownerData, ownerErr := os.ReadFile(rooted(root, apply.OwnershipPath))
	owned := ownerErr == nil && json.Unmarshal(ownerData, &owner) == nil && owner.Marker == apply.OwnershipMarker && owner.Generation == generation
	checks = append(checks, boolCheck("ownership", owned, "managed table ownership matches active generation", StateFailed, "ownership_invalid"))
	return checks
}

func pendingChecks(ctx context.Context, root string, runner nft.Runner) []Check {
	pending, err := apply.Pending(root, "")
	if err != nil {
		return []Check{{ID: "transactions.pending", State: StateUnknown, Summary: "pending transactions could not be inspected", Code: "transaction_inspection_failed", Detail: map[string]any{"error": err.Error()}}}
	}
	if len(pending) == 0 {
		return []Check{{ID: "transactions.pending", State: StateOK, Summary: "no pending transactions"}}
	}
	checks := []Check{{ID: "transactions.pending", State: StatePending, Summary: fmt.Sprintf("%d transaction(s) pending confirmation", len(pending))}}
	mgr := systemd.Manager{Runner: runner}
	for _, tx := range pending {
		detail := map[string]any{"id": tx.ID, "phase": tx.Phase, "deadline": tx.RollbackDeadline}
		unit, unitErr := systemd.RollbackTimer(tx.ID)
		active, timerErr := false, unitErr
		if timerErr == nil {
			active, timerErr = mgr.IsActive(ctx, unit)
		}
		if timerErr != nil {
			checks = append(checks, Check{ID: "transactions.rollback_timer", State: StateUnknown, Summary: "rollback timer could not be inspected", Code: "rollback_timer_inspection_failed", Detail: detail})
		} else if !active {
			checks = append(checks, Check{ID: "transactions.rollback_timer", State: StateFailed, Summary: "pending transaction rollback timer is inactive", Code: "rollback_timer_inactive", Detail: detail})
		} else {
			checks = append(checks, Check{ID: "transactions.rollback_timer", State: StatePending, Summary: "rollback timer is active", Detail: detail})
		}
	}
	return checks
}

func ddnsChecks(ctx context.Context, cfg config.Config, runner nft.Runner) []Check {
	return ddnsChecksFromStatus(ddns.StatusOf(ctx, ddnsConfig(cfg), ddns.NetResolver{}, nftRuntime{runner: runner}))
}

func ddnsChecksFromStatus(status ddns.Status) []Check {
	if !status.Enabled {
		return []Check{{ID: "ddns.config", State: StateNotApplicable, Summary: "DDNS whitelist is disabled"}, {ID: "ddns.runtime", State: StateNotApplicable, Summary: "DDNS runtime check is not applicable"}}
	}
	detail := map[string]any{"hosts": status.Hosts, "configured_ipv4": status.Configured.IPv4, "configured_ipv6": status.Configured.IPv6, "runtime_ipv4": status.Runtime.IPv4, "runtime_ipv6": status.Runtime.IPv6}
	checks := []Check{{ID: "ddns.config", State: StateOK, Summary: fmt.Sprintf("DDNS whitelist enabled with %d host(s)", len(status.Hosts)), Detail: detail}}
	if len(status.Hosts) == 0 {
		checks[0] = Check{ID: "ddns.config", State: StateDegraded, Summary: "DDNS whitelist enabled without hosts", Code: "ddns_hosts_absent"}
	} else if status.ResolutionError != nil {
		detail["error"] = status.ResolutionError.Error()
		checks[0] = Check{ID: "ddns.config", State: StateFailed, Summary: "DDNS hostname resolution failed", Code: "ddns_resolution_failed", Detail: detail}
	}
	if status.RuntimeError != nil {
		checks = append(checks, Check{ID: "ddns.runtime", State: StateUnknown, Summary: "DDNS runtime sets could not be inspected", Code: "ddns_runtime_inspection_failed", Detail: map[string]any{"error": status.RuntimeError.Error()}})
	} else {
		checks = append(checks, Check{ID: "ddns.runtime", State: StateOK, Summary: fmt.Sprintf("runtime has %d IPv4 and %d IPv6 entries", len(status.Runtime.IPv4), len(status.Runtime.IPv6))})
	}
	if status.Stale {
		checks = append(checks, Check{ID: "ddns.freshness", State: StateFailed, Summary: "DDNS runtime metadata is stale", Code: "ddns_runtime_stale"})
	} else if status.MetadataError != nil && !errors.Is(status.MetadataError, os.ErrNotExist) {
		checks = append(checks, Check{ID: "ddns.freshness", State: StateUnknown, Summary: "DDNS metadata could not be read", Code: "ddns_metadata_failed", Detail: map[string]any{"error": status.MetadataError.Error()}})
	}
	return checks
}

func ddnsTimerChecks(ctx context.Context, cfg config.Config, runner nft.Runner) []Check {
	state, err := (systemd.Manager{Runner: runner}).DDNSState(ctx)
	if err != nil {
		return []Check{{ID: "ddns.timer", State: StateUnknown, Summary: "DDNS timer could not be inspected", Code: "ddns_timer_inspection_failed", Detail: map[string]any{"error": err.Error()}}}
	}
	desired := cfg.SSH.DDNSWhitelist.Enabled
	ok := state.Enabled == desired && state.Active == desired
	summary := fmt.Sprintf("DDNS timer desired=%t enabled=%t active=%t", desired, state.Enabled, state.Active)
	return []Check{boolCheck("ddns.timer", ok, summary, StateFailed, "ddns_timer_state_mismatch")}
}

func liveFirewallChecks(ctx context.Context, runner nft.Runner) []Check {
	res := runner.Run(ctx, "nft", "list", "table", "inet", "hostfw")
	if !res.OK() {
		return []Check{{ID: "firewall.table", State: StateUnknown, Summary: "managed table could not be inspected", Code: "nft_inspection_failed", Detail: map[string]any{"error": res.Error().Error()}}, {ID: "firewall.ownership", State: StateUnknown, Summary: "live ownership could not be verified", Code: "nft_inspection_failed"}}
	}
	return []Check{
		{ID: "firewall.table", State: StateOK, Summary: "managed table inet hostfw present"},
		boolCheck("firewall.ownership", strings.Contains(res.Stdout, apply.OwnershipMarker), "live managed table ownership marker present", StateFailed, "live_ownership_invalid"),
	}
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
	if req.BoolFlag("dry-run") {
		var buf bytes.Buffer
		if err := config.Save(&buf, cfg); err != nil {
			return err
		}
		printFiles(io, []apply.File{{Path: configPath(req), Data: buf.Bytes()}})
		return nil
	}
	printConfigReview(io, cfg)
	if !req.BoolFlag("yes") {
		return errors.New("init writes desired config only after local review; rerun with --yes to proceed")
	}
	if err := writeConfig(req, cfg); err != nil {
		return err
	}
	fmt.Fprintln(io.Stdout, "initialized cnftctl desired config; run cnftctl apply to create and load a generation")
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
	_, files, err := generationFiles(req.Flag("root"), cfg)
	if err != nil {
		return err
	}
	if _, err := apply.ValidateCandidate(ctx, apply.Options{Files: files, NftConfigPath: "firewall.nft", Runner: s.runner, DDNSDesired: cfg.SSH.DDNSWhitelist.Enabled}); err != nil {
		checks := []Check{{ID: "config.valid", State: StateOK, Summary: "desired config is valid"}, {ID: "generation.nft_exact", State: StateFailed, Summary: "exact generation candidate failed nft validation", Code: "nft_validation_failed", Detail: map[string]any{"error": err.Error()}}}
		return finishReport(io, req, newReport("validate", checks, nil))
	}
	checks := []Check{{ID: "config.valid", State: StateOK, Summary: "desired config is valid"}, {ID: "generation.manifest", State: StateOK, Summary: "generation manifest is valid"}, {ID: "generation.nft_exact", State: StateOK, Summary: "exact generation candidate is valid"}}
	return finishReport(io, req, newReport("validate", checks, nil))
}

func (s realService) plan(io IO, req CommandRequest) error {
	cfg, err := loadConfig(req)
	if err != nil {
		return err
	}
	generation, files, err := generationFiles(req.Flag("root"), cfg)
	if err != nil {
		return err
	}
	plan, err := apply.PlanFilesWithDDNS(req.Flag("root"), files, cfg.SSH.DDNSWhitelist.Enabled)
	if err != nil {
		return err
	}
	changes := make([]map[string]string, 0, len(plan.Changes))
	for _, ch := range plan.Changes {
		changes = append(changes, map[string]string{"action": ch.Action, "path": ch.Path})
	}
	state, summary := StateOK, "no file or active policy changes"
	if len(changes) > 0 {
		state, summary = StatePending, fmt.Sprintf("%d file change(s) pending", len(changes))
	}
	checks := []Check{{ID: "plan.changes", State: state, Summary: summary}, {ID: "plan.active_policy", State: state, Summary: fmt.Sprintf("active nftables would change: %t", plan.WouldLoadNftables)}}
	return finishReport(io, req, newReport("plan", checks, map[string]any{"generation": generation, "changes": changes, "would_load_nftables": plan.WouldLoadNftables}))
}

func (s realService) apply(ctx context.Context, io IO, req CommandRequest) error {
	if req.Flag("root") == "" && !req.BoolFlag("dry-run") {
		if check := platformCheckAt("/etc/os-release"); check.State != StateOK {
			return fmt.Errorf("production apply refused: %s", check.Summary)
		}
	}
	cfg, err := loadConfig(req)
	if err != nil {
		return err
	}
	_, files, err := generationFiles(req.Flag("root"), cfg)
	if err != nil {
		return err
	}
	if cfg.SSH.DDNSWhitelist.Enabled {
		resolver := s.resolver
		if resolver == nil {
			resolver = ddns.NetResolver{}
		}
		resolved, resolveErr := ddns.Resolve(ctx, ddnsConfig(cfg), resolver)
		if resolveErr != nil {
			return fmt.Errorf("resolve initial DDNS candidate entries: %w", resolveErr)
		}
		files, err = embedDDNSCandidate(files, resolved, cfg.SSH.DDNSWhitelist.TTL.Duration)
		if err != nil {
			return err
		}
	}
	_, files, err = finalizeGeneration(files, cfg.SSH.DDNSWhitelist.Enabled)
	if err != nil {
		return err
	}
	if err := checkSSHSessionCoverage(ctx, cfg, req); err != nil {
		return err
	}
	sshOverrideAcknowledged := req.BoolFlag("acknowledge-ssh-lockout-risk")
	sshOverrideSource, sshOverrideContext := "", ""
	if sshOverrideAcknowledged {
		sshOverrideSource = "cli"
		sshOverrideContext = firstNonEmpty(req.Environment["SSH_CONNECTION"], req.Environment["SSH_CLIENT"])
	}
	tx, plan, err := apply.Apply(ctx, apply.Options{
		Root:                    req.Flag("root"),
		Files:                   files,
		NftConfigPath:           "firewall.nft",
		DryRun:                  req.BoolFlag("dry-run"),
		RequireRoot:             !req.BoolFlag("dry-run") && req.Flag("root") == "",
		RollbackTimeout:         120 * time.Second,
		Runner:                  s.runner,
		Systemd:                 systemd.Manager{Runner: s.runner},
		SSHOverrideAcknowledged: sshOverrideAcknowledged,
		SSHOverrideReason:       req.Flag("reason"),
		SSHOverrideSource:       sshOverrideSource,
		SSHOverrideContext:      sshOverrideContext,
		DDNSDesired:             cfg.SSH.DDNSWhitelist.Enabled,
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
	if tx.Confirmed && tx.ID == "" {
		fmt.Fprintf(io.Stdout, "already active generation=%s; no transaction created\n", tx.Generation)
		return nil
	}
	fmt.Fprintf(io.Stdout, "applied transaction %s generation=%s rollback-deadline=%s\n", tx.ID, tx.Generation, tx.RollbackDeadline.Format(time.RFC3339))
	fmt.Fprintf(io.Stdout, "run cnftctl confirm %s before %s or rollback will restore the previous generation\n", tx.ID, tx.RollbackDeadline.Format(time.RFC3339))
	return nil
}

func (s realService) confirm(ctx context.Context, io IO, req CommandRequest) error {
	id := ""
	if len(req.Args) == 1 {
		id = req.Args[0]
	}
	tx, err := apply.Confirm(ctx, req.Flag("root"), "", id, systemd.Manager{Runner: s.runner})
	if err != nil {
		return err
	}
	fmt.Fprintf(io.Stdout, "confirmed transaction %s\n", tx.ID)
	return nil
}

func (s realService) rollback(ctx context.Context, io IO, req CommandRequest) error {
	id := ""
	if len(req.Args) == 1 {
		id = req.Args[0]
	}
	pending, err := apply.Pending(req.Flag("root"), "")
	if err != nil {
		return err
	}
	if id == "" {
		if len(pending) != 1 {
			return fmt.Errorf("expected one pending transaction, found %d", len(pending))
		}
		id = pending[0].ID
	}
	txDir := rooted(req.Flag("root"), apply.TransactionRoot+"/"+id)
	if err := apply.Restore(ctx, req.Flag("root"), txDir, s.runner); err != nil {
		return err
	}
	fmt.Fprintf(io.Stdout, "rolled back transaction %s\n", id)
	return nil
}

func (s realService) reconcile(ctx context.Context, io IO, req CommandRequest) error {
	if err := apply.Reconcile(ctx, req.Flag("root"), s.runner, systemd.Manager{Runner: s.runner}); err != nil {
		return err
	}
	fmt.Fprintln(io.Stdout, "reconciled durable transactions")
	return nil
}

func (s realService) doctor(ctx context.Context, io IO, req CommandRequest) error {
	return s.status(ctx, io, req)
}

func (s realService) transactionsList(io IO, req CommandRequest) error {
	pending, err := apply.Pending(req.Flag("root"), "")
	if err != nil {
		return err
	}
	items := make([]map[string]any, 0, len(pending))
	for _, tx := range pending {
		items = append(items, map[string]any{"id": tx.ID, "generation": tx.Generation, "phase": tx.Phase, "deadline": tx.RollbackDeadline})
	}
	state, summary := StateOK, "no pending transactions"
	if len(items) > 0 {
		state, summary = StatePending, fmt.Sprintf("%d transaction(s) pending confirmation", len(items))
	}
	return finishReport(io, req, newReport("transactions list", []Check{{ID: "transactions.pending", State: state, Summary: summary}}, map[string]any{"transactions": items}))
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
	if err := writeConfig(req, cfg); err != nil {
		return err
	}
	fmt.Fprintf(io.Stdout, "open %s %s changed=%t\n", res.Entry.Protocol, ports.FormatPort(res.Entry), res.Changed)
	printPortWarnings(io, res.Warnings)
	printApplyGuidance(io, res.Changed)
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
	if err := writeConfig(req, cfg); err != nil {
		return err
	}
	fmt.Fprintf(io.Stdout, "close %s %s changed=%t\n", res.Entry.Protocol, ports.FormatPort(res.Entry), res.Changed)
	printApplyGuidance(io, res.Changed)
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
	if err := writeConfig(req, cfg); err != nil {
		return err
	}
	fmt.Fprintf(io.Stdout, "whitelist add %s changed=%t\n", res.Entry.Prefix, res.Changed)
	for _, w := range res.Warnings {
		fmt.Fprintf(io.Stderr, "warning: %s\n", w.Message)
	}
	printApplyGuidance(io, res.Changed)
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
	if err := writeConfig(req, cfg); err != nil {
		return err
	}
	fmt.Fprintf(io.Stdout, "whitelist remove %s changed=%t\n", res.Entry.Prefix, res.Changed)
	printApplyGuidance(io, res.Changed)
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
	if err := writeConfig(req, cfg); err != nil {
		return err
	}
	fmt.Fprintf(io.Stdout, "DDNS whitelist enabled changed=%t\n", changed)
	printApplyGuidance(io, changed)
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
	if err := writeConfig(req, cfg); err != nil {
		return err
	}
	fmt.Fprintf(io.Stdout, "DDNS whitelist disabled changed=%t\n", changed)
	printApplyGuidance(io, changed)
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
	if err := writeConfig(req, cfg); err != nil {
		return err
	}
	fmt.Fprintf(io.Stdout, "DDNS host add changed=%t\n", changed)
	printApplyGuidance(io, changed)
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
	if err := writeConfig(req, cfg); err != nil {
		return err
	}
	fmt.Fprintf(io.Stdout, "DDNS host remove changed=%t\n", changed)
	printApplyGuidance(io, changed)
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
	if err := writeConfig(req, cfg); err != nil {
		return err
	}
	fmt.Fprintf(io.Stdout, "DDNS IPv6 prefix length set to /%d changed=%t\n", prefixLen, changed)
	printApplyGuidance(io, changed)
	return nil
}

func (s realService) ddnsRefresh(ctx context.Context, io IO, req CommandRequest) error {
	cfg, err := loadConfig(req)
	if err != nil {
		return err
	}
	metadataPath := rooted(req.Flag("root"), ddns.MetadataPath)
	metadata, loadErr := ddns.LoadMetadata(metadataPath)
	if loadErr != nil && !errors.Is(loadErr, os.ErrNotExist) {
		return fmt.Errorf("load DDNS metadata: %w", loadErr)
	}
	var result ddns.RefreshResult
	refreshErr := apply.WithFirewallLock(req.Flag("root"), func() error {
		var err error
		result, err = ddns.Refresh(ctx, ddnsConfig(cfg), ddns.NetResolver{}, nftRuntime{runner: s.runner})
		return err
	})
	_, metadataErr := ddns.RecordAttempt(metadataPath, metadata, result, cfg.SSH.DDNSWhitelist.TTL.Duration, refreshErr, time.Now())
	if metadataErr != nil {
		return fmt.Errorf("persist DDNS refresh metadata: %w", metadataErr)
	}
	if refreshErr != nil {
		return refreshErr
	}
	fmt.Fprintf(io.Stdout, "refreshed DDNS whitelist: %d IPv4 entries, %d IPv6 prefixes\n", len(result.IPv4), len(result.IPv6))
	return nil
}

func (s realService) ddnsStatus(ctx context.Context, io IO, req CommandRequest) error {
	cfg, err := loadConfig(req)
	if err != nil {
		return err
	}
	if req.Flag("root") != "" {
		data := map[string]any{"enabled": cfg.SSH.DDNSWhitelist.Enabled, "host_count": len(cfg.SSH.DDNSWhitelist.Hosts), "ipv6_prefix_len": cfg.SSH.DDNSWhitelist.IPv6PrefixLen}
		checks := []Check{{ID: "ddns.runtime", State: StateNotApplicable, Summary: "runtime DDNS inspection is unavailable for an alternate root", Code: "offline_root"}}
		return finishReport(io, req, newReport("ddns status", checks, data))
	}
	status := ddns.StatusOf(ctx, ddnsConfig(cfg), ddns.NetResolver{}, nftRuntime{runner: s.runner}, rooted(req.Flag("root"), ddns.MetadataPath))
	data := map[string]any{"enabled": status.Enabled, "host_count": len(status.Hosts), "ipv6_prefix_len": status.IPv6PrefixLen, "configured_ipv4_count": len(status.Configured.IPv4), "configured_ipv6_count": len(status.Configured.IPv6), "runtime_ipv4_count": len(status.Runtime.IPv4), "runtime_ipv6_count": len(status.Runtime.IPv6), "metadata": status.Metadata, "stale": status.Stale}
	if req.BoolFlag("detail") {
		data["hosts"], data["configured"], data["runtime"] = status.Hosts, status.Configured, status.Runtime
	}
	return finishReport(io, req, newReport("ddns status", ddnsChecksFromStatus(status), data))
}

func (s realService) ddnsTimerStatus(ctx context.Context, io IO) error {
	mgr := systemd.Manager{Runner: s.runner}
	state, err := mgr.DDNSState(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(io.Stdout, "%s enabled: %t active: %t\n", systemd.DDNSRefreshTimer, state.Enabled, state.Active)
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
	if err := writeConfig(req, cfg); err != nil {
		return err
	}
	fmt.Fprintf(io.Stdout, "SSH mode set to %s changed=%t\n", mode, res.Changed)
	for _, w := range res.Warnings {
		fmt.Fprintf(io.Stderr, "warning: %s\n", w.Message)
	}
	printApplyGuidance(io, res.Changed)
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
	if err := writeConfig(req, cfg); err != nil {
		return err
	}
	fmt.Fprintf(io.Stdout, "feature %s changed=%t\n", req.Args[0], res.Changed)
	for _, w := range res.Warnings {
		fmt.Fprintf(io.Stderr, "warning: %s\n", w.Message)
	}
	printApplyGuidance(io, res.Changed)
	return nil
}

func (s realService) dockerStatus(io IO, req CommandRequest) error {
	backend, ok, err := inspectDockerBackend(req.Flag("root"), req.Flag("daemon-json"))
	if err != nil {
		return err
	}
	if ok {
		fmt.Fprintf(io.Stdout, "firewall-backend: %s\n", backend)
	} else {
		fmt.Fprintln(io.Stdout, "firewall-backend: not set")
	}
	return nil
}

func (s realService) dockerBackend(ctx context.Context, io IO, req CommandRequest, write bool) error {
	requestedPath := firstNonEmpty(req.Flag("daemon-json"), "/etc/docker/daemon.json")
	path := rooted(req.Flag("root"), requestedPath)
	if write {
		var err error
		path, err = managedPath(req.Flag("root"), requestedPath, true)
		if err != nil {
			return err
		}
	}
	before, err := os.ReadFile(path)
	beforeExists := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	plan, err := docker.PlanNftablesBackend(path, before, beforeExists, time.Now().UTC())
	if err != nil {
		return err
	}
	if !plan.Changed {
		if req.Flag("root") == "" {
			if err := validateDockerDaemonConfig(ctx, s.runner, before); err != nil {
				return err
			}
		}
		fmt.Fprintf(io.Stdout, "%s already uses firewall-backend=nftables\n", plan.Path)
		return nil
	}
	fmt.Fprintf(io.Stdout, "would update %s\n", plan.Path)
	fmt.Fprintf(io.Stdout, "backup path: %s\n", plan.BackupPath)
	for _, warning := range plan.Warnings {
		fmt.Fprintf(io.Stderr, "warning: %s\n", warning.Message)
	}
	if !write {
		if req.Flag("root") == "" {
			if err := validateDockerDaemonConfig(ctx, s.runner, plan.After); err != nil {
				return err
			}
			fmt.Fprintln(io.Stdout, "Docker daemon accepts the proposed configuration")
		}
		fmt.Fprintln(io.Stdout, "dry-run: no Docker daemon files written")
		return nil
	}
	if !req.BoolFlag("yes") {
		return errors.New("writing Docker daemon configuration requires --yes; cnftctl will not restart Docker")
	}
	if req.Flag("root") == "" {
		if err := validateDockerDaemonConfig(ctx, s.runner, plan.After); err != nil {
			return err
		}
	}
	if err := docker.WritePlan(plan); err != nil {
		return err
	}
	fmt.Fprintf(io.Stdout, "updated %s; restart Docker manually when ready\n", plan.Path)
	return nil
}

func validateDockerDaemonConfig(ctx context.Context, runner nft.Runner, data []byte) error {
	if runner == nil {
		return errors.New("docker daemon configuration validation is unavailable")
	}
	staged, err := os.CreateTemp("", "cnftctl-daemon.*.json")
	if err != nil {
		return fmt.Errorf("stage Docker daemon configuration for validation: %w", err)
	}
	path := staged.Name()
	defer os.Remove(path)
	if err := staged.Chmod(0o600); err != nil {
		_ = staged.Close()
		return fmt.Errorf("protect staged Docker daemon configuration: %w", err)
	}
	if _, err := staged.Write(data); err != nil {
		_ = staged.Close()
		return fmt.Errorf("stage Docker daemon configuration for validation: %w", err)
	}
	if err := staged.Sync(); err != nil {
		_ = staged.Close()
		return fmt.Errorf("sync staged Docker daemon configuration: %w", err)
	}
	if err := staged.Close(); err != nil {
		return fmt.Errorf("close staged Docker daemon configuration: %w", err)
	}

	result := runner.Run(ctx, "dockerd", "--validate", "--config-file="+path)
	if result.OK() {
		return nil
	}
	detail := strings.TrimSpace(result.Stderr)
	if detail == "" {
		detail = strings.TrimSpace(result.Stdout)
	}
	if detail == "" {
		detail = result.Err.Error()
	}
	return fmt.Errorf("docker daemon rejected proposed configuration; no file was written: %s", detail)
}

func (s realService) adoptReference(io IO, req CommandRequest) error {
	adoption, err := install.AdoptReference(req.Flag("root"))
	if err != nil {
		return err
	}
	cfg := config.Default()
	for _, p := range adoption.OpenPorts {
		entry, err := ports.ParseEntry(p.Protocol, p.Port, "adopted from reference")
		if err != nil {
			return err
		}
		cfg.OpenPorts = append(cfg.OpenPorts, config.OpenPort{Protocol: entry.Protocol, Port: int(entry.Start), EndPort: int(entry.End), Comment: entry.Comment})
	}
	for _, value := range adoption.Whitelist.IPv4 {
		cfg.SSH.StaticWhitelist.IPv4 = append(cfg.SSH.StaticWhitelist.IPv4, config.WhitelistEntry{Value: value, Comment: "adopted from reference"})
	}
	for _, value := range adoption.Whitelist.IPv6 {
		cfg.SSH.StaticWhitelist.IPv6 = append(cfg.SSH.StaticWhitelist.IPv6, config.WhitelistEntry{Value: value, Comment: "adopted from reference"})
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	for _, warning := range adoption.Warnings {
		fmt.Fprintf(io.Stderr, "warning: %s\n", warning)
	}
	printConfigReview(io, cfg)
	if req.BoolFlag("dry-run") {
		var buf bytes.Buffer
		if err := config.Save(&buf, cfg); err != nil {
			return err
		}
		printFiles(io, []apply.File{{Path: configPath(req), Data: buf.Bytes()}})
		return nil
	}
	if !req.BoolFlag("yes") {
		return errors.New("adoption writes desired config only after local review; rerun with --yes to proceed")
	}
	if err := writeConfig(req, cfg); err != nil {
		return err
	}
	fmt.Fprintln(io.Stdout, "adopted reference desired config; run cnftctl apply to create and load a generation")
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
	path, err := managedPath(req.Flag("root"), configPath(req), false)
	if err != nil {
		return config.Config{}, err
	}
	cfg, err := config.LoadFile(path)
	if err != nil {
		return config.Config{}, fmt.Errorf("load config %s: %w", path, err)
	}
	return cfg, nil
}

func writeConfig(req CommandRequest, cfg config.Config) error {
	path, err := managedPath(req.Flag("root"), configPath(req), true)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := config.Save(&buf, cfg); err != nil {
		return err
	}
	if before, readErr := os.ReadFile(path); readErr == nil && bytes.Equal(before, buf.Bytes()) {
		return nil
	}
	return config.SaveFile(path, cfg, 0o600)
}

func configPath(req CommandRequest) string {
	if p := req.Flag("config"); p != "" {
		return p
	}
	return defaultConfigPath
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

func generationFiles(root string, cfg config.Config) (string, []apply.File, error) {
	seed, err := renderApplyFiles(cfg)
	if err != nil {
		return "", nil, err
	}
	return finalizeGeneration(seed, cfg.SSH.DDNSWhitelist.Enabled)
}

func finalizeGeneration(files []apply.File, ddnsDesired bool) (string, []apply.File, error) {
	return apply.FinalizeFiles(files, ddnsDesired)
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
				IPv4: whitelistValues(cfg.SSH.StaticWhitelist.IPv4),
				IPv6: whitelistValues(cfg.SSH.StaticWhitelist.IPv6),
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
	for _, value := range append(append([]config.WhitelistEntry{}, cfg.SSH.StaticWhitelist.IPv4...), cfg.SSH.StaticWhitelist.IPv6...) {
		entry, err := whitelist.ParseEntry(value.Value, value.Comment)
		if err == nil {
			entries = append(entries, entry)
		}
	}
	return entries
}

func whitelistValues(entries []config.WhitelistEntry) []string {
	values := make([]string, 0, len(entries))
	for _, entry := range entries {
		values = append(values, entry.Value)
	}
	return values
}

func checkSSHSessionCoverage(ctx context.Context, cfg config.Config, req CommandRequest) error {
	if cfg.SSH.Mode == "open" {
		return nil
	}
	connection := strings.Fields(req.Environment["SSH_CONNECTION"])
	if len(connection) < 1 {
		connection = strings.Fields(req.Environment["SSH_CLIENT"])
	}
	if len(connection) < 1 {
		return nil
	}
	client, err := netip.ParseAddr(connection[0])
	if err != nil {
		return fmt.Errorf("parse current SSH client address: %w", err)
	}
	covered := false
	for _, entry := range append(append([]config.WhitelistEntry{}, cfg.SSH.StaticWhitelist.IPv4...), cfg.SSH.StaticWhitelist.IPv6...) {
		if prefix, err := netip.ParsePrefix(entry.Value); err == nil && prefix.Contains(client) {
			covered = true
		}
		if addr, err := netip.ParseAddr(entry.Value); err == nil && addr == client {
			covered = true
		}
	}
	if !covered && cfg.SSH.DDNSWhitelist.Enabled {
		resolver := ddns.NetResolver{}
		for _, host := range cfg.SSH.DDNSWhitelist.Hosts {
			ipv4, _ := resolver.LookupA(ctx, host)
			ipv6, _ := resolver.LookupAAAA(ctx, host)
			for _, addr := range append(ipv4, ipv6...) {
				if addr == client || addr.Is6() && client.Is6() && netip.PrefixFrom(addr, cfg.SSH.DDNSWhitelist.IPv6PrefixLen).Masked().Contains(client) {
					covered = true
				}
			}
		}
	}
	if !covered && cfg.TrustedInterfaces.Enabled {
		var server netip.Addr
		if len(connection) >= 3 {
			server, _ = netip.ParseAddr(connection[2])
		}
		for _, iface := range cfg.TrustedInterfaces.Interfaces {
			networkInterface, err := net.InterfaceByName(iface)
			if err != nil {
				continue
			}
			addresses, err := networkInterface.Addrs()
			if err != nil {
				continue
			}
			for _, address := range addresses {
				prefix, err := netip.ParsePrefix(address.String())
				if err == nil && server.IsValid() && prefix.Addr() == server {
					covered = true
				}
			}
		}
	}
	if covered {
		return nil
	}
	if !req.BoolFlag("acknowledge-ssh-lockout-risk") {
		return fmt.Errorf("current SSH client %s is not covered by the effective candidate policy; apply requires --acknowledge-ssh-lockout-risk", client)
	}
	if strings.TrimSpace(req.Flag("reason")) == "" {
		return errors.New("--reason is required for SSH lockout-risk acknowledgement")
	}
	return nil
}

func setWhitelistConfig(cfg *config.Config, wc whitelist.Config) {
	cfg.SSH.StaticWhitelist.IPv4 = nil
	cfg.SSH.StaticWhitelist.IPv6 = nil
	for _, entry := range whitelist.List(&wc) {
		value := config.WhitelistEntry{Value: prefixText(entry.Prefix), Comment: entry.Comment}
		if entry.Prefix.Addr().Is4() {
			cfg.SSH.StaticWhitelist.IPv4 = append(cfg.SSH.StaticWhitelist.IPv4, value)
		} else {
			cfg.SSH.StaticWhitelist.IPv6 = append(cfg.SSH.StaticWhitelist.IPv6, value)
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
			StaticWhitelist: append(whitelistValues(cfg.SSH.StaticWhitelist.IPv4), whitelistValues(cfg.SSH.StaticWhitelist.IPv6)...),
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
	return nft.ReplaceSets(ctx, r.runner, "inet", "hostfw", []nft.SetReplacement{
		{Set: "ddns_whitelist_v4", Elements: timedElements(addrStrings(ipv4), ttl)},
		{Set: "ddns_whitelist_v6", Elements: timedElements(prefixStrings(ipv6), ttl)},
	})
}

func embedDDNSCandidate(files []apply.File, result ddns.RefreshResult, ttl time.Duration) ([]apply.File, error) {
	for i := range files {
		if filepath.Base(files[i].Path) != "firewall.nft" {
			continue
		}
		text := string(files[i].Data)
		v4 := "set ddns_whitelist_v4 { type ipv4_addr; flags timeout; timeout " + nftTimeout(ttl) + "; }"
		v6 := "set ddns_whitelist_v6 { type ipv6_addr; flags interval, timeout; timeout " + nftTimeout(ttl) + "; }"
		text = strings.Replace(text, v4, setWithElements(v4, addrStrings(result.IPv4), ttl), 1)
		text = strings.Replace(text, v6, setWithElements(v6, prefixStrings(result.IPv6), ttl), 1)
		if !strings.Contains(text, "ddns_whitelist_v4") {
			return nil, errors.New("generated candidate has no DDNS sets")
		}
		files[i].Data = []byte(text)
		return files, nil
	}
	return nil, errors.New("generated candidate firewall.nft not found")
}

func setWithElements(declaration string, values []string, ttl time.Duration) string {
	if len(values) == 0 {
		return declaration
	}
	return strings.TrimSuffix(declaration, " }") + " elements = { " + strings.Join(timedElements(values, ttl), ", ") + " }; }"
}
func timedElements(values []string, ttl time.Duration) []string {
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = value + " timeout " + nftTimeout(ttl)
	}
	return out
}
func addrStrings(values []netip.Addr) []string {
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = value.String()
	}
	return out
}
func prefixStrings(values []netip.Prefix) []string {
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = value.String()
	}
	return out
}

func (r nftRuntime) List(ctx context.Context) (ddns.RuntimeEntries, error) {
	var entries ddns.RuntimeEntries
	res := r.runner.Run(ctx, "nft", "--json", "list", "set", "inet", "hostfw", "ddns_whitelist_v4")
	if !res.OK() {
		return entries, res.Error()
	}
	ipv4, err := parseNftSetElements(res.Stdout)
	if err != nil {
		return entries, err
	}
	for _, value := range ipv4 {
		addr, err := netip.ParseAddr(value)
		if err != nil || !addr.Is4() {
			continue
		}
		entries.IPv4 = append(entries.IPv4, addr.Unmap())
	}

	res = r.runner.Run(ctx, "nft", "--json", "list", "set", "inet", "hostfw", "ddns_whitelist_v6")
	if !res.OK() {
		return entries, res.Error()
	}
	ipv6, err := parseNftSetElements(res.Stdout)
	if err != nil {
		return entries, err
	}
	for _, value := range ipv6 {
		prefix, err := netip.ParsePrefix(value)
		if err == nil && prefix.Addr().Is6() {
			entries.IPv6 = append(entries.IPv6, prefix.Masked())
			continue
		}
		addr, err := netip.ParseAddr(value)
		if err == nil && addr.Is6() && !addr.Is4() {
			entries.IPv6 = append(entries.IPv6, netip.PrefixFrom(addr, addr.BitLen()))
		}
	}
	return entries, nil
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

func printApplyGuidance(io IO, changed bool) {
	if changed {
		fmt.Fprintln(io.Stdout, "run cnftctl apply to load active policy")
	}
}

func printConfigReview(io IO, cfg config.Config) {
	fmt.Fprintf(io.Stdout, "config version: %d\n", cfg.Version)
	fmt.Fprintf(io.Stdout, "WAN interface: %s\n", firstNonEmpty(cfg.WANInterface, "default eth0"))
	fmt.Fprintf(io.Stdout, "open ports: %d\n", len(cfg.OpenPorts))
	fmt.Fprintf(io.Stdout, "SSH mode: %s\n", cfg.SSH.Mode)
	fmt.Fprintf(io.Stdout, "DDNS whitelist: enabled=%t hosts=%d\n", cfg.SSH.DDNSWhitelist.Enabled, len(cfg.SSH.DDNSWhitelist.Hosts))
	fmt.Fprintf(io.Stdout, "trusted interfaces: enabled=%t interfaces=%d\n", cfg.TrustedInterfaces.Enabled, len(cfg.TrustedInterfaces.Interfaces))
	fmt.Fprintf(io.Stdout, "Docker integration: enabled=%t\n", cfg.Docker.Enabled)
	risks := cfg.RiskExplanations()
	if len(risks) == 0 {
		fmt.Fprintln(io.Stdout, "risk warnings: none")
		return
	}
	fmt.Fprintln(io.Stdout, "risk warnings:")
	for _, risk := range risks {
		fmt.Fprintf(io.Stdout, "- %s\n", risk)
	}
}

func renderedInSync(root string, cfg config.Config) (bool, error) {
	desired, _, err := generationFiles(root, cfg)
	if err != nil {
		return false, err
	}
	target, err := os.Readlink(rooted(root, apply.ActiveSelector))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	active := filepath.Base(target)
	if !cfg.SSH.DDNSWhitelist.Enabled {
		return active == desired, nil
	}
	genDir := rooted(root, apply.GenerationRoot+"/"+active)
	var manifest apply.Manifest
	manifestData, err := os.ReadFile(filepath.Join(genDir, "manifest.json"))
	if err != nil || json.Unmarshal(manifestData, &manifest) != nil || manifest.Version != 1 || !manifest.DDNSDesired {
		return false, errors.New("active DDNS generation manifest is invalid")
	}
	files := make([]apply.File, 0, len(manifest.Files))
	for _, entry := range manifest.Files {
		name := filepath.Base(entry.Path)
		if name != entry.Path {
			return false, errors.New("active DDNS generation manifest contains an unsafe path")
		}
		data, err := os.ReadFile(filepath.Join(genDir, name))
		if err != nil {
			return false, err
		}
		files = append(files, apply.File{Path: apply.GenerationRoot + "/" + active + "/" + name, Mode: os.FileMode(entry.Mode), Data: data})
	}
	intent, err := ddnsIntentGeneration(files)
	return err == nil && intent == desired, err
}

func ddnsIntentGeneration(files []apply.File) (string, error) {
	normalized := append([]apply.File(nil), files...)
	for i := range normalized {
		if filepath.Base(normalized[i].Path) == "firewall.nft" {
			normalized[i].Data = stripDDNSCandidateElements(normalized[i].Data)
		}
	}
	generation, _, err := apply.FinalizeFiles(normalized, true)
	return generation, err
}

func stripDDNSCandidateElements(data []byte) []byte {
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if !strings.Contains(line, "set ddns_whitelist_v4 ") && !strings.Contains(line, "set ddns_whitelist_v6 ") {
			continue
		}
		if start := strings.Index(line, " elements = {"); start >= 0 && strings.HasSuffix(line, " }; }") {
			lines[i] = line[:start] + " }"
		}
	}
	return []byte(strings.Join(lines, "\n"))
}

func inspectDockerBackend(root, path string) (string, bool, error) {
	path = rooted(root, firstNonEmpty(path, "/etc/docker/daemon.json"))
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		data = nil
	} else if err != nil {
		return "", false, err
	}
	cfg, err := docker.InspectDaemonJSON(path, data)
	if err != nil {
		return "", false, err
	}
	backend, kind := docker.FirewallBackend(cfg)
	if kind == docker.BackendInvalid {
		return "", false, errors.New("daemon.json firewall-backend must be a string")
	}
	return backend, kind == docker.BackendString, nil
}

func parseNftSetElements(data string) ([]string, error) {
	if strings.TrimSpace(data) == "" {
		return nil, nil
	}
	var doc any
	if err := json.Unmarshal([]byte(data), &doc); err != nil {
		return nil, err
	}
	var values []string
	walkNftJSON(doc, &values)
	return values, nil
}

func walkNftJSON(value any, out *[]string) {
	switch v := value.(type) {
	case map[string]any:
		if elem, ok := v["elem"]; ok {
			walkNftElement(elem, out)
		}
		for key, child := range v {
			if key == "elem" {
				continue
			}
			walkNftJSON(child, out)
		}
	case []any:
		for _, child := range v {
			walkNftJSON(child, out)
		}
	}
}

func walkNftElement(value any, out *[]string) {
	switch v := value.(type) {
	case string:
		*out = append(*out, v)
	case []any:
		for _, child := range v {
			walkNftElement(child, out)
		}
	case map[string]any:
		if elem, ok := v["elem"]; ok {
			walkNftElement(elem, out)
		}
		if prefix, ok := v["prefix"].(map[string]any); ok {
			addr, _ := prefix["addr"].(string)
			length, ok := numberString(prefix["len"])
			if addr != "" && ok {
				*out = append(*out, addr+"/"+length)
			}
		}
		if val, ok := v["val"]; ok {
			walkNftElement(val, out)
		}
	}
}

func numberString(value any) (string, bool) {
	switch v := value.(type) {
	case float64:
		return strconv.Itoa(int(v)), true
	case string:
		return v, true
	default:
		return "", false
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

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func managedPath(root, path string, createParent bool) (string, error) {
	if root == "" {
		return path, nil
	}
	if !filepath.IsAbs(root) || !filepath.IsAbs(path) {
		return "", errors.New("alternate root and managed path must be absolute")
	}
	cleanRoot := filepath.Clean(root)
	for _, component := range strings.Split(path, string(filepath.Separator)) {
		if component == ".." {
			return "", fmt.Errorf("managed path %q contains a parent traversal component", path)
		}
	}
	rel := strings.TrimPrefix(path, string(filepath.Separator))
	if rel == "" || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return "", fmt.Errorf("managed path %q escapes alternate root", path)
	}
	rootReal, err := filepath.EvalSymlinks(cleanRoot)
	if err != nil {
		return "", fmt.Errorf("resolve alternate root: %w", err)
	}
	parent := filepath.Join(rootReal, filepath.Dir(rel))
	if createParent {
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return "", err
		}
	}
	parentReal, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return "", fmt.Errorf("resolve managed parent: %w", err)
	}
	within, err := filepath.Rel(rootReal, parentReal)
	if err != nil || within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("managed path %q escapes alternate root through a symlink", path)
	}
	target := filepath.Join(parentReal, filepath.Base(rel))
	targetReal, err := filepath.EvalSymlinks(target)
	if err == nil {
		within, relErr := filepath.Rel(rootReal, targetReal)
		if relErr != nil || within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("managed path %q escapes alternate root through a symlink", path)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("resolve managed path: %w", err)
	}
	return target, nil
}

func validateExecutionMode(req CommandRequest) error {
	if req.Flag("root") == "" {
		return nil
	}
	forbidden := map[string]bool{"confirm": true, "rollback": true, "reconcile": true, "ddns refresh": true, "ddns timer status": true}
	if forbidden[req.Command] || req.Command == "apply" && !req.BoolFlag("dry-run") {
		return fmt.Errorf("%s is unavailable with --root; alternate roots are strictly offline", req.Command)
	}
	return nil
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
