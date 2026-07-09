package apply

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/calmcacil/cnftctl/internal/nft"
	"github.com/calmcacil/cnftctl/internal/systemd"
)

const DefaultRollbackTimeout = 120 * time.Second

type File struct {
	Path string
	Mode os.FileMode
	Data []byte
}

type Change struct {
	Path   string
	Action string
	Before []byte
	After  []byte
}

type Plan struct {
	Changes           []Change
	WouldLoadNftables bool
}

type Options struct {
	Root            string
	RunRoot         string
	Files           []File
	NftConfigPath   string
	DryRun          bool
	RequireRoot     bool
	EUID            func() int
	RollbackTimeout time.Duration
	Runner          nft.Runner
	Systemd         systemd.Manager
	Now             func() time.Time
}

type Transaction struct {
	ID               string    `json:"id"`
	StartedAt        time.Time `json:"started_at"`
	RollbackDeadline time.Time `json:"rollback_deadline"`
	Confirmed        bool      `json:"confirmed"`
	RolledBack       bool      `json:"rolled_back"`
	Files            []Record  `json:"files"`
	NftConfigPath    string    `json:"nft_config_path"`
	UnitName         string    `json:"unit_name"`
}

type Record struct {
	Path       string      `json:"path"`
	BackupPath string      `json:"backup_path,omitempty"`
	Existed    bool        `json:"existed"`
	Mode       os.FileMode `json:"mode"`
}

func PlanFiles(root string, files []File, nftConfigPath string) (Plan, error) {
	var plan Plan
	for _, file := range normalizedFiles(files) {
		if file.Path == "" {
			return plan, errors.New("file path is required")
		}
		abs := underRoot(root, file.Path)
		before, err := os.ReadFile(abs)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return plan, err
		}
		if err == nil && string(before) == string(file.Data) {
			continue
		}
		action := "create"
		if err == nil {
			action = "update"
		}
		plan.Changes = append(plan.Changes, Change{Path: file.Path, Action: action, Before: before, After: file.Data})
	}
	plan.WouldLoadNftables = nftConfigPath != "" && len(plan.Changes) > 0
	return plan, nil
}

func Apply(ctx context.Context, opts Options) (Transaction, Plan, error) {
	if opts.RequireRoot && euid(opts) != 0 {
		return Transaction{}, Plan{}, errors.New("root privileges are required")
	}
	if opts.RollbackTimeout == 0 {
		opts.RollbackTimeout = DefaultRollbackTimeout
	}
	if opts.RollbackTimeout != DefaultRollbackTimeout {
		return Transaction{}, Plan{}, fmt.Errorf("rollback timeout must be %s", DefaultRollbackTimeout)
	}
	plan, err := PlanFiles(opts.Root, opts.Files, opts.NftConfigPath)
	if err != nil || opts.DryRun {
		return Transaction{}, plan, err
	}
	if opts.NftConfigPath == "" {
		return Transaction{}, plan, errors.New("nft config path is required")
	}
	runner := opts.Runner
	if runner == nil {
		runner = nft.ExecRunner{}
	}
	if err := nft.ValidateFile(ctx, runner, underRoot(opts.Root, opts.NftConfigPath)); err != nil {
		return Transaction{}, plan, err
	}
	runRoot := runRoot(opts.Root, opts.RunRoot)
	if err := os.MkdirAll(runRoot, 0o700); err != nil {
		return Transaction{}, plan, err
	}
	lock, err := acquireLock(runRoot)
	if err != nil {
		return Transaction{}, plan, err
	}
	defer lock.Close()

	tx := Transaction{ID: newID(), StartedAt: now(opts), NftConfigPath: opts.NftConfigPath}
	tx.RollbackDeadline = tx.StartedAt.Add(opts.RollbackTimeout)
	tx.UnitName = "cnftctl-rollback-" + tx.ID
	txDir := filepath.Join(runRoot, tx.ID)
	backupDir := filepath.Join(txDir, "backup")
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		return Transaction{}, plan, err
	}
	if err := backupFiles(opts.Root, backupDir, normalizedFiles(opts.Files), &tx); err != nil {
		return Transaction{}, plan, err
	}
	if err := writeState(txDir, tx); err != nil {
		return Transaction{}, plan, err
	}
	if err := writeRollbackScript(txDir); err != nil {
		return Transaction{}, plan, err
	}
	for _, file := range normalizedFiles(opts.Files) {
		if err := atomicWrite(underRoot(opts.Root, file.Path), file.Data, fileMode(file)); err != nil {
			_ = Restore(ctx, opts.Root, txDir, runner)
			return Transaction{}, plan, err
		}
	}
	if err := nft.LoadFile(ctx, runner, underRoot(opts.Root, opts.NftConfigPath)); err != nil {
		_ = Restore(ctx, opts.Root, txDir, runner)
		return Transaction{}, plan, err
	}
	mgr := opts.Systemd
	if mgr.Runner == nil {
		mgr.Runner = runner
	}
	if err := mgr.StartRollback(ctx, tx.UnitName, filepath.Join(txDir, "rollback.sh"), opts.RollbackTimeout); err != nil {
		_ = Restore(ctx, opts.Root, txDir, runner)
		return Transaction{}, plan, err
	}
	if err := writeState(txDir, tx); err != nil {
		return Transaction{}, plan, err
	}
	return tx, plan, nil
}

func Confirm(ctx context.Context, root, runRootPath, transactionID string, mgr systemd.Manager) (Transaction, error) {
	txDir, err := transactionDir(root, runRootPath, transactionID)
	if err != nil {
		return Transaction{}, err
	}
	tx, err := readState(txDir)
	if err != nil {
		return Transaction{}, err
	}
	if tx.RolledBack {
		return tx, errors.New("transaction already rolled back")
	}
	if mgr.Runner == nil {
		mgr.Runner = nft.ExecRunner{}
	}
	if err := mgr.Cancel(ctx, tx.UnitName); err != nil {
		return tx, err
	}
	tx.Confirmed = true
	if err := writeState(txDir, tx); err != nil {
		return tx, err
	}
	_ = os.Remove(filepath.Join(runRoot(root, runRootPath), "apply.lock"))
	return tx, nil
}

func Restore(ctx context.Context, root, txDir string, runner nft.Runner) error {
	tx, err := readState(txDir)
	if err != nil {
		return err
	}
	for _, rec := range tx.Files {
		path := underRoot(root, rec.Path)
		if !rec.Existed {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			continue
		}
		data, err := os.ReadFile(rec.BackupPath)
		if err != nil {
			return err
		}
		if err := atomicWrite(path, data, rec.Mode); err != nil {
			return err
		}
	}
	if runner == nil {
		runner = nft.ExecRunner{}
	}
	if tx.NftConfigPath != "" {
		if err := nft.LoadFile(ctx, runner, underRoot(root, tx.NftConfigPath)); err != nil {
			return err
		}
	}
	tx.RolledBack = true
	if err := writeState(txDir, tx); err != nil {
		return err
	}
	_ = os.Remove(filepath.Join(filepath.Dir(txDir), "apply.lock"))
	return nil
}

func Pending(root, runRootPath string) ([]Transaction, error) {
	base := runRoot(root, runRootPath)
	entries, err := os.ReadDir(base)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Transaction
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		tx, err := readState(filepath.Join(base, entry.Name()))
		if err == nil && !tx.Confirmed && !tx.RolledBack {
			out = append(out, tx)
		}
	}
	return out, nil
}

func acquireLock(runRoot string) (*os.File, error) {
	path := filepath.Join(runRoot, "apply.lock")
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return nil, errors.New("another apply transaction is pending")
	}
	return file, err
}

func backupFiles(root, backupDir string, files []File, tx *Transaction) error {
	for _, file := range files {
		path := underRoot(root, file.Path)
		info, err := os.Stat(path)
		rec := Record{Path: file.Path, Mode: fileMode(file)}
		if err == nil {
			rec.Existed = true
			rec.Mode = info.Mode().Perm()
			rec.BackupPath = filepath.Join(backupDir, strings.TrimPrefix(filepath.Clean(file.Path), string(filepath.Separator)))
			if err := os.MkdirAll(filepath.Dir(rec.BackupPath), 0o700); err != nil {
				return err
			}
			if err := copyFile(path, rec.BackupPath, rec.Mode); err != nil {
				return err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		tx.Files = append(tx.Files, rec)
	}
	return nil
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".cnftctl-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func writeState(txDir string, tx Transaction) error {
	data, err := json.MarshalIndent(tx, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return atomicWrite(filepath.Join(txDir, "state.json"), data, 0o600)
}

func readState(txDir string) (Transaction, error) {
	data, err := os.ReadFile(filepath.Join(txDir, "state.json"))
	if err != nil {
		return Transaction{}, err
	}
	var tx Transaction
	if err := json.Unmarshal(data, &tx); err != nil {
		return Transaction{}, err
	}
	return tx, nil
}

func writeRollbackScript(txDir string) error {
	script := "#!/bin/sh\nset -eu\ncnftctl rollback --transaction-dir \"$(dirname \"$0\")\"\n"
	return atomicWrite(filepath.Join(txDir, "rollback.sh"), []byte(script), 0o700)
}

func transactionDir(root, runRootPath, transactionID string) (string, error) {
	if transactionID == "" {
		pending, err := Pending(root, runRootPath)
		if err != nil {
			return "", err
		}
		if len(pending) != 1 {
			return "", fmt.Errorf("expected one pending transaction, found %d", len(pending))
		}
		transactionID = pending[0].ID
	}
	return filepath.Join(runRoot(root, runRootPath), transactionID), nil
}

func normalizedFiles(files []File) []File {
	out := append([]File(nil), files...)
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func fileMode(file File) os.FileMode {
	if file.Mode == 0 {
		return 0o644
	}
	return file.Mode
}

func underRoot(root, path string) string {
	if root == "" {
		return path
	}
	clean := filepath.Clean(path)
	if filepath.IsAbs(clean) {
		clean = strings.TrimPrefix(clean, string(filepath.Separator))
	}
	return filepath.Join(root, clean)
}

func runRoot(root, path string) string {
	if path == "" {
		path = "/run/cnftctl"
	}
	return underRoot(root, path)
}

func now(opts Options) time.Time {
	if opts.Now != nil {
		return opts.Now()
	}
	return time.Now().UTC()
}

func euid(opts Options) int {
	if opts.EUID != nil {
		return opts.EUID()
	}
	return os.Geteuid()
}

func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
