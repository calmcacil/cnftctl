package apply

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/calmcacil/cnftctl/internal/nft"
	"github.com/calmcacil/cnftctl/internal/systemd"
)

const (
	DefaultRollbackTimeout = 120 * time.Second
	StateRoot              = "/var/lib/cnftctl"
	GenerationRoot         = StateRoot + "/generations"
	TransactionRoot        = StateRoot + "/transactions"
	ActiveSelector         = StateRoot + "/active"
	OwnershipPath          = StateRoot + "/ownership.json"
	RunRoot                = "/run/cnftctl"
	OwnershipMarker        = "cnftctl-owned:inet-hostfw:v1"
	generationPlaceholder  = "/var/lib/cnftctl/generations/{generation}"
)

type Phase string

const (
	PhasePrepared   Phase = "prepared"
	PhaseArmed      Phase = "rollback-armed"
	PhaseActivating Phase = "activating"
	PhaseActivated  Phase = "activated"
	PhaseConfirmed  Phase = "confirmed"
	PhaseRolledBack Phase = "rolled-back"
)

type File struct {
	Path string
	Mode os.FileMode
	Data []byte
}
type Change struct {
	Path, Action  string
	Before, After []byte
}
type Plan struct {
	Changes           []Change
	WouldLoadNftables bool
	Generation        string
}
type Options struct {
	Root, RunRoot                 string
	Files                         []File
	NftConfigPath, ExecutablePath string
	DryRun, RequireRoot           bool
	EUID                          func() int
	RollbackTimeout               time.Duration
	Runner                        nft.Runner
	Systemd                       systemd.Manager
	Now                           func() time.Time
	SSHOverrideAcknowledged       bool
	SSHOverrideReason             string
	SSHOverrideSource             string
	SSHOverrideContext            string
	DDNSDesired                   bool
}
type ManifestEntry struct {
	Path   string `json:"path"`
	Mode   uint32 `json:"mode"`
	SHA256 string `json:"sha256"`
	Size   int    `json:"size"`
}
type Manifest struct {
	Version     int             `json:"version"`
	Files       []ManifestEntry `json:"files"`
	DDNSDesired bool            `json:"ddns_desired"`
}
type Transaction struct {
	ID                 string            `json:"id"`
	Phase              Phase             `json:"phase"`
	StartedAt          time.Time         `json:"started_at"`
	RollbackDeadline   time.Time         `json:"rollback_deadline"`
	Generation         string            `json:"generation"`
	PreviousGeneration string            `json:"previous_generation"`
	FreshInstall       bool              `json:"fresh_install"`
	FirewallWasEnabled bool              `json:"firewall_was_enabled"`
	Confirmed          bool              `json:"confirmed"`
	RolledBack         bool              `json:"rolled_back"`
	SSHOverride        *SSHOverrideAudit `json:"ssh_override,omitempty"`
}
type SSHOverrideAudit struct {
	Acknowledged bool   `json:"acknowledged"`
	Reason       string `json:"reason"`
	Source       string `json:"source,omitempty"`
	Context      string `json:"context,omitempty"`
}
type ownership struct{ Marker, Generation string }

var idPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

func PlanFiles(root string, files []File, _ string) (Plan, error) {
	return PlanFilesWithDDNS(root, files, false)
}

func PlanFilesWithDDNS(root string, files []File, ddnsDesired bool) (Plan, error) {
	generation, finalized, err := FinalizeFiles(files, ddnsDesired)
	if err != nil {
		return Plan{}, err
	}
	plan := Plan{Generation: generation}
	genDir := rooted(root, GenerationRoot+"/"+generation)
	for _, f := range finalized {
		path := rooted(root, f.Path)
		if isGenerationFile(f) {
			path = filepath.Join(genDir, filepath.Base(f.Path))
		}
		before, readErr := os.ReadFile(path)
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			return plan, readErr
		}
		if readErr == nil && string(before) == string(f.Data) {
			continue
		}
		action := "create"
		if readErr == nil {
			action = "update"
		}
		plan.Changes = append(plan.Changes, Change{Path: path, Action: action, Before: before, After: f.Data})
	}
	plan.WouldLoadNftables = len(plan.Changes) > 0
	return plan, nil
}

// FinalizeFiles computes identity from semantic placeholders, then materializes exact immutable bytes.
func FinalizeFiles(files []File, ddnsDesired bool) (string, []File, error) {
	semantic, _, err := buildSemanticManifest(files)
	if err != nil {
		return "", nil, err
	}
	semantic.DDNSDesired = ddnsDesired
	generation, err := manifestHash(semantic)
	if err != nil {
		return "", nil, err
	}
	finalized := append([]File(nil), files...)
	for i := range finalized {
		data := normalizeGenerationReferences(finalized[i].Data)
		data = bytes.ReplaceAll(data, []byte(generationPlaceholder), []byte(GenerationRoot+"/"+generation))
		data = bytes.ReplaceAll(data, []byte(OwnershipMarker+":generation:{generation}"), []byte(OwnershipMarker+":generation:"+generation))
		finalized[i].Data = data
		if isGenerationFile(finalized[i]) {
			finalized[i].Path = GenerationRoot + "/" + generation + "/" + filepath.Base(finalized[i].Path)
		}
	}
	return generation, finalized, nil
}

func isGenerationFile(f File) bool {
	return strings.Contains(f.Path, "/generations/") || strings.Contains(string(f.Data), GenerationRoot+"/")
}

func Apply(ctx context.Context, opts Options) (tx Transaction, plan Plan, err error) {
	if opts.RequireRoot && euid(opts) != 0 {
		return tx, plan, errors.New("root privileges are required")
	}
	if opts.RollbackTimeout == 0 {
		opts.RollbackTimeout = DefaultRollbackTimeout
	}
	if opts.RollbackTimeout != DefaultRollbackTimeout {
		return tx, plan, fmt.Errorf("rollback timeout must be %s", DefaultRollbackTimeout)
	}
	sshOverride, err := validateSSHOverride(opts)
	if err != nil {
		return tx, plan, err
	}
	generation, files, err := FinalizeFiles(opts.Files, opts.DDNSDesired)
	if err != nil {
		return tx, plan, err
	}
	manifest, files, err := buildManifest(files)
	if err != nil {
		return tx, plan, err
	}
	manifest.DDNSDesired = opts.DDNSDesired
	plan, err = PlanFilesWithDDNS(opts.Root, files, opts.DDNSDesired)
	if err != nil || opts.DryRun {
		return tx, plan, err
	}
	runner := opts.Runner
	if runner == nil {
		runner = nft.ExecRunner{}
	}
	mgr := opts.Systemd
	if mgr.Runner == nil {
		mgr.Runner = runner
	}
	lock, err := lockRuntime(rooted(opts.Root, first(opts.RunRoot, RunRoot)))
	if err != nil {
		return tx, plan, err
	}
	defer lock.Close()
	pending, err := Pending(opts.Root, "")
	if err != nil {
		return tx, plan, err
	}
	if len(pending) > 0 {
		return tx, plan, errors.New("another apply transaction is pending")
	}
	active, err := activeGeneration(opts.Root)
	if err != nil {
		return tx, plan, err
	}
	if active == generation {
		if err = verifyGeneration(rooted(opts.Root, GenerationRoot+"/"+generation), manifest); err != nil {
			return tx, plan, err
		}
		if opts.Root == "" {
			if err = verifyInstalledAssets(); err != nil {
				return tx, plan, err
			}
		}
		if err = verifyStoredOwnership(opts.Root, generation); err != nil {
			return tx, plan, err
		}
		table, present, liveErr := nft.ListTable(ctx, runner, "inet", "hostfw")
		if liveErr != nil {
			return tx, plan, liveErr
		}
		if present && !hasExactGenerationMarker(table, generation) {
			return tx, plan, errors.New("existing inet hostfw failed exact generation marker verification")
		}
		ddnsState, stateErr := mgr.DDNSState(ctx)
		if stateErr != nil {
			return tx, plan, stateErr
		}
		if present && ddnsState.Enabled == opts.DDNSDesired && ddnsState.Active == opts.DDNSDesired {
			return Transaction{Generation: generation, Phase: PhaseConfirmed, Confirmed: true}, Plan{Generation: generation}, nil
		}
		if present {
			if err = mgr.ReconcileDDNSTimer(ctx, opts.DDNSDesired); err != nil {
				return tx, plan, err
			}
			return Transaction{Generation: generation, Phase: PhaseConfirmed, Confirmed: true}, Plan{Generation: generation}, nil
		}
		active = ""
	}
	if opts.Root == "" {
		if err = verifyInstalledAssets(); err != nil {
			return tx, plan, err
		}
	}
	if err = checkOwnership(ctx, opts.Root, runner, active); err != nil {
		return tx, plan, err
	}
	if _, err = ValidateCandidate(ctx, Options{Files: files, NftConfigPath: opts.NftConfigPath, Runner: runner, DDNSDesired: opts.DDNSDesired}); err != nil {
		return tx, plan, err
	}
	genDir := rooted(opts.Root, GenerationRoot+"/"+generation)
	if err = writeGeneration(genDir, manifest, files); err != nil {
		return tx, plan, err
	}
	firewallState, err := mgr.FirewallState(ctx)
	if err != nil {
		return tx, plan, err
	}
	tx = Transaction{ID: newID(), Phase: PhasePrepared, StartedAt: now(opts), Generation: generation, PreviousGeneration: active, FreshInstall: active == "", FirewallWasEnabled: firewallState.Enabled, SSHOverride: sshOverride}
	tx.RollbackDeadline = tx.StartedAt.Add(opts.RollbackTimeout)
	txDir := rooted(opts.Root, TransactionRoot+"/"+tx.ID)
	if err = writeTransaction(txDir, tx); err != nil {
		return tx, plan, err
	}
	if err = mgr.ArmRollback(ctx, tx.ID); err != nil {
		return tx, plan, recoverApply(ctx, opts.Root, tx, mgr, runner, err)
	}
	tx.Phase = PhaseArmed
	if err = writeTransaction(txDir, tx); err != nil {
		return tx, plan, recoverApply(ctx, opts.Root, tx, mgr, runner, err)
	}
	tx.Phase = PhaseActivating
	if err = writeTransaction(txDir, tx); err != nil {
		return tx, plan, recoverApply(ctx, opts.Root, tx, mgr, runner, err)
	}
	if err = setActive(opts.Root, generation); err != nil {
		return tx, plan, recoverApply(ctx, opts.Root, tx, mgr, runner, err)
	}
	if err = writeJSON(rooted(opts.Root, OwnershipPath), ownership{Marker: OwnershipMarker, Generation: generation}, 0o600); err != nil {
		return tx, plan, recoverApply(ctx, opts.Root, tx, mgr, runner, err)
	}
	if err = mgr.ActivateFirewall(ctx); err != nil {
		return tx, plan, recoverApply(ctx, opts.Root, tx, mgr, runner, err)
	}
	tx.Phase = PhaseActivated
	if err = writeTransaction(txDir, tx); err != nil {
		return tx, plan, recoverApply(ctx, opts.Root, tx, mgr, runner, err)
	}
	if err = mgr.ReconcileDDNSTimer(ctx, opts.DDNSDesired); err != nil {
		return tx, plan, recoverApply(ctx, opts.Root, tx, mgr, runner, err)
	}
	if err = mgr.SetFirewallEnabled(ctx, true); err != nil {
		return tx, plan, recoverApply(ctx, opts.Root, tx, mgr, runner, err)
	}
	return tx, plan, nil
}

// ValidateCandidate validates exact final-shaped generation bytes without changing durable state.
func ValidateCandidate(ctx context.Context, opts Options) (string, error) {
	manifest, files, err := buildManifest(opts.Files)
	if err != nil {
		return "", err
	}
	manifest.DDNSDesired = opts.DDNSDesired
	semantic, _, err := buildSemanticManifest(files)
	if err != nil {
		return "", err
	}
	semantic.DDNSDesired = opts.DDNSDesired
	generation, err := manifestHash(semantic)
	if err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp("", "cnftctl-candidate-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	candidateDir := filepath.Join(tmp, generation)
	if err = writeGeneration(candidateDir, manifest, files); err != nil {
		return "", err
	}
	runner := opts.Runner
	if runner == nil {
		runner = nft.ExecRunner{}
	}
	if err = nft.ValidateFile(ctx, runner, filepath.Join(candidateDir, configName(opts.NftConfigPath))); err != nil {
		return "", fmt.Errorf("validate exact generation candidate: %w", err)
	}
	return generation, nil
}

func Confirm(ctx context.Context, root, _, id string, mgr systemd.Manager) (Transaction, error) {
	lock, err := lockRuntime(rooted(root, RunRoot))
	if err != nil {
		return Transaction{}, err
	}
	defer lock.Close()
	txDir, err := transactionDir(root, id)
	if err != nil {
		return Transaction{}, err
	}
	tx, err := readTransaction(txDir)
	if err != nil {
		return tx, err
	}
	if tx.RolledBack {
		return tx, errors.New("transaction already rolled back")
	}
	if tx.Confirmed {
		return tx, mgr.CancelRollback(ctx, tx.ID)
	}
	if tx.Phase != PhaseActivated {
		return tx, errors.New("only an activated transaction can be confirmed")
	}
	verifyRunner := mgr.Runner
	if verifyRunner == nil {
		verifyRunner = nft.ExecRunner{}
	}
	if err = verifyActivated(ctx, root, tx, verifyRunner); err != nil {
		return tx, err
	}
	tx.Confirmed = true
	tx.Phase = PhaseConfirmed
	if err = writeTransaction(txDir, tx); err != nil {
		return tx, err
	}
	if err = mgr.CancelRollback(ctx, tx.ID); err != nil {
		return tx, err
	}
	return tx, nil
}

// Restore is retained for the rollback command, but only accepts a durable transaction directory.
func Restore(ctx context.Context, root, txDir string, runner nft.Runner) error {
	lock, err := lockRuntime(rooted(root, RunRoot))
	if err != nil {
		return err
	}
	defer lock.Close()
	clean := filepath.Clean(txDir)
	base := rooted(root, TransactionRoot)
	if filepath.Dir(clean) != base || !idPattern.MatchString(filepath.Base(clean)) {
		return errors.New("transaction directory is not a durable cnftctl transaction")
	}
	return rollback(ctx, root, clean, runner)
}

// WithFirewallLock serializes standalone firewall mutations with apply and rollback.
func WithFirewallLock(root string, fn func() error) error {
	lock, err := lockRuntime(rooted(root, RunRoot))
	if err != nil {
		return err
	}
	defer lock.Close()
	return fn()
}
func Reconcile(ctx context.Context, root string, runner nft.Runner, managers ...systemd.Manager) error {
	lock, err := lockRuntime(rooted(root, RunRoot))
	if err != nil {
		return err
	}
	defer lock.Close()
	pending, err := Pending(root, "")
	if err != nil {
		return err
	}
	var errs []error
	for _, tx := range pending {
		errs = append(errs, rollbackWithManager(ctx, root, rooted(root, TransactionRoot+"/"+tx.ID), runner, manager(runner, managers)))
	}
	return errors.Join(errs...)
}
func rollback(ctx context.Context, root, txDir string, runner nft.Runner) error {
	return rollbackWithManager(ctx, root, txDir, runner, systemd.Manager{Runner: runner})
}
func rollbackWithManager(ctx context.Context, root, txDir string, runner nft.Runner, mgr systemd.Manager) error {
	tx, err := readTransaction(txDir)
	if err != nil {
		return err
	}
	if tx.Confirmed || tx.RolledBack {
		return nil
	}
	if runner == nil {
		runner = nft.ExecRunner{}
	}
	var actionErrs []error
	mutationsPossible := tx.Phase == PhaseActivating || tx.Phase == PhaseActivated
	if tx.FreshInstall {
		if !mutationsPossible {
			tx.RolledBack = true
			tx.Phase = PhaseRolledBack
			return writeTransaction(txDir, tx)
		}
		table, present, tableErr := nft.ListTable(ctx, runner, "inet", "hostfw")
		if tableErr != nil {
			actionErrs = append(actionErrs, tableErr)
		} else if present && !hasGenerationMarker(table, tx.Generation) {
			actionErrs = append(actionErrs, errors.New("live table is not owned by transaction generation"))
		} else if present {
			actionErrs = append(actionErrs, nft.DeleteTable(ctx, runner, "inet", "hostfw"))
		}
		active, activeErr := activeGeneration(root)
		if activeErr != nil {
			actionErrs = append(actionErrs, activeErr)
		} else if active == tx.Generation || active == "" {
			if removeErr := os.Remove(rooted(root, ActiveSelector)); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				actionErrs = append(actionErrs, removeErr)
			}
		} else {
			actionErrs = append(actionErrs, errors.New("active selector no longer belongs to transaction"))
		}
		var own ownership
		ownErr := readJSON(rooted(root, OwnershipPath), &own)
		if errors.Is(ownErr, os.ErrNotExist) {
			ownErr = nil
		} else if ownErr == nil && own.Marker == OwnershipMarker && own.Generation == tx.Generation {
			ownErr = os.Remove(rooted(root, OwnershipPath))
		} else if ownErr == nil {
			ownErr = errors.New("ownership no longer belongs to transaction")
		}
		if ownErr != nil {
			actionErrs = append(actionErrs, ownErr)
		}
	} else if mutationsPossible {
		stage, err := updateRollbackStage(ctx, root, tx, runner)
		if err != nil {
			return err
		}
		if stage < 1 {
			if err := setActive(root, tx.PreviousGeneration); err != nil {
				return err
			}
		}
		if stage < 2 {
			if err := nft.LoadFile(ctx, runner, rooted(root, GenerationRoot+"/"+tx.PreviousGeneration+"/firewall.nft")); err != nil {
				return err
			}
		}
		if stage < 3 {
			if err := writeJSON(rooted(root, OwnershipPath), ownership{Marker: OwnershipMarker, Generation: tx.PreviousGeneration}, 0o600); err != nil {
				return err
			}
		}
		if stage, err := updateRollbackStage(ctx, root, tx, runner); err != nil {
			return err
		} else if stage != 3 {
			return errors.New("previous generation runtime state was not fully restored")
		}
	}
	if err := mgr.SetFirewallEnabled(ctx, tx.FirewallWasEnabled); err != nil {
		actionErrs = append(actionErrs, err)
	}
	desired, desiredErr := generationDDNSDesired(root, tx.PreviousGeneration)
	if tx.FreshInstall {
		desired, desiredErr = false, nil
	}
	if desiredErr != nil {
		actionErrs = append(actionErrs, desiredErr)
	} else if err := mgr.ReconcileDDNSTimer(ctx, desired); err != nil {
		actionErrs = append(actionErrs, err)
	}
	if actionErr := errors.Join(actionErrs...); actionErr != nil {
		return actionErr
	}
	tx.RolledBack = true
	tx.Phase = PhaseRolledBack
	return writeTransaction(txDir, tx)
}

// updateRollbackStage recognizes only the monotonic states written by update rollback.
func updateRollbackStage(ctx context.Context, root string, tx Transaction, runner nft.Runner) (int, error) {
	active, err := activeGeneration(root)
	if err != nil {
		return 0, err
	}
	var own ownership
	if err := readJSON(rooted(root, OwnershipPath), &own); err != nil {
		return 0, errors.New("cannot verify rollback ownership")
	}
	table, present, err := nft.ListTable(ctx, runner, "inet", "hostfw")
	if err != nil {
		return 0, err
	}
	markers := generationMarker.FindAllString(table, -1)
	if !present || len(markers) != 1 {
		return 0, errors.New("cannot verify rollback live generation")
	}
	live := strings.TrimPrefix(markers[0], OwnershipMarker+":generation:")
	current, previous := tx.Generation, tx.PreviousGeneration
	states := [][3]string{
		{current, current, current},
		{previous, current, current},
		{previous, current, previous},
		{previous, previous, previous},
	}
	actual := [3]string{active, own.Generation, live}
	if own.Marker == OwnershipMarker {
		for stage, state := range states {
			if actual == state {
				return stage, nil
			}
		}
	}
	return 0, errors.New("rollback runtime ownership is inconsistent or belongs to another generation")
}
func Pending(root, _ string) ([]Transaction, error) {
	base := rooted(root, TransactionRoot)
	entries, err := os.ReadDir(base)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Transaction
	for _, e := range entries {
		if !e.IsDir() || !idPattern.MatchString(e.Name()) {
			continue
		}
		tx, e2 := readTransaction(filepath.Join(base, e.Name()))
		if e2 == nil && !tx.Confirmed && !tx.RolledBack {
			out = append(out, tx)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.Before(out[j].StartedAt) })
	return out, nil
}

func buildManifest(files []File) (Manifest, []File, error) {
	m := Manifest{Version: 1}
	seen := map[string]bool{}
	out := append([]File(nil), files...)
	sort.Slice(out, func(i, j int) bool { return filepath.Base(out[i].Path) < filepath.Base(out[j].Path) })
	for _, f := range out {
		name := filepath.Base(filepath.Clean(f.Path))
		if name == "." || name == string(filepath.Separator) || seen[name] {
			return m, nil, fmt.Errorf("invalid or duplicate generation file %q", f.Path)
		}
		seen[name] = true
		mode := f.Mode
		if mode == 0 {
			mode = 0o644
		}
		sum := sha256.Sum256(f.Data)
		m.Files = append(m.Files, ManifestEntry{Path: name, Mode: uint32(mode.Perm()), SHA256: hex.EncodeToString(sum[:]), Size: len(f.Data)})
	}
	return m, out, nil
}
func buildSemanticManifest(files []File) (Manifest, []File, error) {
	normalized := append([]File(nil), files...)
	for i := range normalized {
		normalized[i].Data = normalizeGenerationReferences(normalized[i].Data)
	}
	return buildManifest(normalized)
}

func generationDDNSDesired(root, generation string) (bool, error) {
	if generation == "" {
		return false, nil
	}
	var manifest Manifest
	if err := readJSON(rooted(root, GenerationRoot+"/"+generation+"/manifest.json"), &manifest); err != nil {
		return false, fmt.Errorf("read previous generation DDNS state: %w", err)
	}
	if manifest.Version != 1 {
		return false, errors.New("previous generation manifest version is incompatible")
	}
	return manifest.DDNSDesired, nil
}

func manager(runner nft.Runner, managers []systemd.Manager) systemd.Manager {
	if len(managers) > 0 {
		return managers[0]
	}
	return systemd.Manager{Runner: runner}
}

func verifyInstalledAssets() error { return verifyInstalledAssetsAt("") }
func verifyInstalledAssetsAt(root string) error {
	assets := []string{
		"/usr/bin/cnftctl",
		"/usr/lib/cnftctl/cnftctl-recover",
		"/usr/lib/systemd/system/cnftctl-firewall.service",
		"/usr/lib/systemd/system/cnftctl-reconcile.service",
		"/usr/lib/systemd/system/cnftctl-rollback@.service",
		"/usr/lib/systemd/system/cnftctl-rollback@.timer",
		"/usr/lib/systemd/system/cnftctl-ddns-refresh.service",
		"/usr/lib/systemd/system/cnftctl-ddns-refresh.timer",
		"/var/lib/cnftctl/delivery/manifest",
		"/var/lib/cnftctl/delivery/SHA256SUMS",
	}
	for _, path := range assets {
		st, err := os.Stat(rooted(root, path))
		if err != nil || !st.Mode().IsRegular() {
			return fmt.Errorf("required installed asset %s is unavailable", path)
		}
	}
	st, err := os.Stat(rooted(root, "/usr/bin/cnftctl"))
	if err != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0o111 == 0 {
		return errors.New("required installed executable /usr/bin/cnftctl is unavailable or not executable")
	}
	checks := map[string]string{
		"/usr/lib/systemd/system/cnftctl-firewall.service":  "ExecStart=/usr/bin/nft -f /var/lib/cnftctl/active/firewall.nft",
		"/usr/lib/systemd/system/cnftctl-reconcile.service": "ExecStart=/usr/bin/cnftctl reconcile",
		"/usr/lib/systemd/system/cnftctl-rollback@.service": "ExecStart=/usr/bin/cnftctl rollback %i",
	}
	for path, required := range checks {
		data, readErr := os.ReadFile(rooted(root, path))
		if readErr != nil || !strings.Contains(string(data), required) {
			return fmt.Errorf("installed unit %s is incompatible", path)
		}
	}
	manifest, err := os.ReadFile(rooted(root, "/var/lib/cnftctl/delivery/manifest"))
	if err != nil || !strings.Contains(string(manifest), "format=1\n") || !strings.Contains(string(manifest), "product=cnftctl\n") {
		return errors.New("installed cnftctl manifest is incompatible")
	}
	return verifyInstalledInventory(root, assets[:len(assets)-2])
}

func verifyInstalledInventory(root string, required []string) error {
	data, err := os.ReadFile(rooted(root, "/var/lib/cnftctl/delivery/SHA256SUMS"))
	if err != nil {
		return err
	}
	installed := map[string]string{"bin/cnftctl": "/usr/bin/cnftctl", "manifest": "/var/lib/cnftctl/delivery/manifest"}
	installed["scripts/cnftctl-recover"] = "/usr/lib/cnftctl/cnftctl-recover"
	for _, path := range required {
		if strings.HasPrefix(path, "/usr/lib/systemd/system/") {
			installed["systemd/"+filepath.Base(path)] = path
		}
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		parts := strings.Split(line, "  ")
		if len(parts) != 2 || len(parts[0]) != 64 || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(parts[0]) || filepath.Clean(parts[1]) != parts[1] || strings.HasPrefix(parts[1], "/") || strings.HasPrefix(parts[1], "../") || seen[parts[1]] {
			return errors.New("installed SHA256SUMS is malformed")
		}
		seen[parts[1]] = true
		if path, ok := installed[parts[1]]; ok {
			b, readErr := os.ReadFile(rooted(root, path))
			if readErr != nil {
				return readErr
			}
			sum := sha256.Sum256(b)
			if hex.EncodeToString(sum[:]) != parts[0] {
				return fmt.Errorf("installed asset %s hash mismatch", path)
			}
		}
	}
	for bundlePath, path := range installed {
		if !seen[bundlePath] {
			return fmt.Errorf("installed SHA256SUMS does not cover %s", path)
		}
	}
	return nil
}

var generationReference = regexp.MustCompile(`/var/lib/cnftctl/generations/[a-f0-9]{64}`)
var generationMarker = regexp.MustCompile(`cnftctl-owned:inet-hostfw:v1:generation:[a-f0-9]{64}`)

func normalizeGenerationReferences(data []byte) []byte {
	data = generationReference.ReplaceAll(data, []byte(generationPlaceholder))
	return generationMarker.ReplaceAll(data, []byte(OwnershipMarker+":generation:{generation}"))
}

func hasGenerationMarker(table, generation string) bool {
	return strings.Contains(table, OwnershipMarker+":generation:"+generation)
}

func hasExactGenerationMarker(table, generation string) bool {
	markers := generationMarker.FindAllString(table, -1)
	return len(markers) == 1 && markers[0] == OwnershipMarker+":generation:"+generation
}

func verifyStoredOwnership(root, generation string) error {
	var own ownership
	if err := readJSON(rooted(root, OwnershipPath), &own); err != nil || own.Marker != OwnershipMarker || own.Generation != generation {
		return errors.New("ownership does not match active generation")
	}
	return nil
}

func validateSSHOverride(opts Options) (*SSHOverrideAudit, error) {
	reason := strings.TrimSpace(opts.SSHOverrideReason)
	source := strings.TrimSpace(opts.SSHOverrideSource)
	context := strings.TrimSpace(opts.SSHOverrideContext)
	if !opts.SSHOverrideAcknowledged {
		if reason != "" || source != "" || context != "" {
			return nil, errors.New("SSH override metadata requires acknowledgement")
		}
		return nil, nil
	}
	if reason == "" {
		return nil, errors.New("SSH override acknowledgement reason is required")
	}
	if len(reason) > 1024 || len(source) > 256 || len(context) > 2048 {
		return nil, errors.New("SSH override audit metadata is too long")
	}
	return &SSHOverrideAudit{Acknowledged: true, Reason: reason, Source: source, Context: context}, nil
}
func manifestHash(m Manifest) (string, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
func writeGeneration(dir string, m Manifest, files []File) error {
	if st, err := os.Lstat(dir); err == nil {
		if !st.IsDir() {
			return errors.New("generation path is not a directory")
		}
		return verifyGeneration(dir, m)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := mkdirSafe(filepath.Dir(dir), 0o700); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(dir), ".generation-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	for _, f := range files {
		mode := f.Mode
		if mode == 0 {
			mode = 0o644
		}
		if err := durableWrite(filepath.Join(stage, filepath.Base(f.Path)), f.Data, mode); err != nil {
			return err
		}
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	if err := durableWrite(filepath.Join(stage, "manifest.json"), append(b, '\n'), 0o444); err != nil {
		return err
	}
	if err := syncDir(stage); err != nil {
		return err
	}
	if err := os.Chmod(stage, 0o500); err != nil {
		return err
	}
	if err := os.Rename(stage, dir); err != nil {
		return err
	}
	return syncDir(filepath.Dir(dir))
}
func verifyGeneration(dir string, m Manifest) error {
	st, err := os.Lstat(dir)
	if err != nil || !st.IsDir() || st.Mode().Perm() != 0o500 {
		return errors.New("immutable generation directory identity or mode mismatch")
	}
	var disk Manifest
	if err := readJSON(filepath.Join(dir, "manifest.json"), &disk); err != nil {
		return err
	}
	if !reflect.DeepEqual(disk, m) {
		return errors.New("immutable generation manifest identity mismatch")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	if len(entries) != len(m.Files)+1 {
		return errors.New("immutable generation contains unexpected files")
	}
	for _, e := range m.Files {
		st, err := os.Lstat(filepath.Join(dir, e.Path))
		if err != nil || !st.Mode().IsRegular() || uint32(st.Mode().Perm()) != e.Mode || st.Size() != int64(e.Size) {
			return fmt.Errorf("immutable generation file %s identity or mode mismatch", e.Path)
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Path))
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		if hex.EncodeToString(sum[:]) != e.SHA256 {
			return fmt.Errorf("immutable generation file %s hash mismatch", e.Path)
		}
	}
	return nil
}

func verifyActivated(ctx context.Context, root string, tx Transaction, r nft.Runner) error {
	if err := verifyTransactionOwnership(root, tx); err != nil {
		return err
	}
	table, present, err := nft.ListTable(ctx, r, "inet", "hostfw")
	if err != nil || !present || !hasGenerationMarker(table, tx.Generation) {
		return errors.New("live managed-table marker verification failed")
	}
	return nil
}

func verifyTransactionOwnership(root string, tx Transaction) error {
	active, err := activeGeneration(root)
	if err != nil || active != tx.Generation {
		return errors.New("active selector does not match transaction generation")
	}
	var own ownership
	if err := readJSON(rooted(root, OwnershipPath), &own); err != nil || own.Marker != OwnershipMarker || own.Generation != tx.Generation {
		return errors.New("ownership does not match transaction generation")
	}
	return nil
}
func checkOwnership(ctx context.Context, root string, r nft.Runner, active string) error {
	table, present, err := nft.ListTable(ctx, r, "inet", "hostfw")
	if err != nil {
		return err
	}
	if !present {
		return nil
	}
	var own ownership
	if err = readJSON(rooted(root, OwnershipPath), &own); err != nil {
		return errors.New("existing inet hostfw is not owned by cnftctl")
	}
	if own.Marker != OwnershipMarker || active == "" || own.Generation != active || !hasGenerationMarker(table, active) {
		return errors.New("existing inet hostfw failed cnftctl ownership verification")
	}
	return nil
}
func activeGeneration(root string) (string, error) {
	target, err := os.Readlink(rooted(root, ActiveSelector))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	name := filepath.Base(target)
	if len(name) != 64 {
		return "", errors.New("invalid active generation selector")
	}
	return name, nil
}
func setActive(root, generation string) error {
	if len(generation) != 64 {
		return errors.New("invalid generation hash")
	}
	path := rooted(root, ActiveSelector)
	if err := mkdirSafe(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".new"
	_ = os.Remove(tmp)
	if err := os.Symlink(filepath.Join("generations", generation), tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	return syncDir(filepath.Dir(path))
}
func recoverApply(ctx context.Context, root string, tx Transaction, mgr systemd.Manager, r nft.Runner, cause error) error {
	txDir := rooted(root, TransactionRoot+"/"+tx.ID)
	if stateErr := writeTransaction(txDir, tx); stateErr != nil {
		return errors.Join(cause, stateErr)
	}
	rollbackErr := rollbackWithManager(ctx, root, txDir, r, mgr)
	if rollbackErr != nil {
		return errors.Join(cause, rollbackErr)
	}
	return errors.Join(cause, mgr.CancelRollback(ctx, tx.ID))
}
func transactionDir(root, id string) (string, error) {
	if id == "" {
		p, err := Pending(root, "")
		if err != nil {
			return "", err
		}
		if len(p) != 1 {
			return "", fmt.Errorf("expected one pending transaction, found %d", len(p))
		}
		id = p[0].ID
	}
	if !idPattern.MatchString(id) {
		return "", errors.New("invalid transaction ID")
	}
	return rooted(root, TransactionRoot+"/"+id), nil
}
func writeTransaction(dir string, tx Transaction) error {
	parent := filepath.Dir(dir)
	if err := mkdirSafe(parent, 0o700); err != nil {
		return err
	}
	created := false
	if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		created = true
	}
	if err := mkdirSafe(dir, 0o700); err != nil {
		return err
	}
	if created {
		if err := syncDir(parent); err != nil {
			return err
		}
	}
	return writeJSON(filepath.Join(dir, "state.json"), tx, 0o600)
}
func readTransaction(dir string) (Transaction, error) {
	var tx Transaction
	err := readJSON(filepath.Join(dir, "state.json"), &tx)
	if err == nil && (!idPattern.MatchString(tx.ID) || tx.ID != filepath.Base(dir)) {
		return tx, errors.New("transaction identity mismatch")
	}
	return tx, err
}
func writeJSON(path string, v any, mode os.FileMode) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return durableWrite(path, append(b, '\n'), mode)
}
func readJSON(path string, v any) error {
	st, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return errors.New("refusing symlink state file")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
func durableWrite(path string, data []byte, mode os.FileMode) error {
	if err := mkdirSafe(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if st, err := os.Lstat(path); err == nil && st.Mode()&os.ModeSymlink != 0 {
		return errors.New("refusing to replace symlink")
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".cnftctl-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err = f.Write(data); err == nil {
		err = f.Chmod(mode)
	}
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(tmp, path); err != nil {
		return err
	}
	return syncDir(filepath.Dir(path))
}
func mkdirSafe(path string, mode os.FileMode) error {
	if err := os.MkdirAll(path, mode); err != nil {
		return err
	}
	st, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return errors.New("state path is not a safe directory")
	}
	return nil
}
func syncDir(path string) error {
	d, err := os.Open(path)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
func lockRuntime(path string) (*os.File, error) {
	if err := mkdirSafe(path, 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(path, "apply.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, errors.New("another cnftctl operation is running")
	}
	return f, nil
}
func rooted(root, path string) string {
	if root == "" {
		return filepath.Clean(path)
	}
	return filepath.Join(root, strings.TrimPrefix(filepath.Clean(path), "/"))
}
func configName(path string) string {
	if path == "" {
		return "firewall.nft"
	}
	return filepath.Base(path)
}
func first(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
func now(o Options) time.Time {
	if o.Now != nil {
		return o.Now().UTC()
	}
	return time.Now().UTC()
}
func euid(o Options) int {
	if o.EUID != nil {
		return o.EUID()
	}
	return os.Geteuid()
}
func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}
