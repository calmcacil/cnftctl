package apply

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/calmcacil/cnftctl/internal/nft"
	"github.com/calmcacil/cnftctl/internal/systemd"
)

type call struct {
	name string
	args []string
}
type fakeRunner struct {
	calls   []call
	results []nft.Result
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) nft.Result {
	f.calls = append(f.calls, call{name: name, args: append([]string(nil), args...)})
	if len(f.results) == 0 {
		return nft.Result{}
	}
	res := f.results[0]
	f.results = f.results[1:]
	return res
}

func TestDryRunPlansWithoutWritingOrCommands(t *testing.T) {
	root := t.TempDir()
	r := &fakeRunner{}
	_, plan, err := Apply(context.Background(), Options{Root: root, DryRun: true, Runner: r, NftConfigPath: "/etc/nftables.conf", Files: []File{{Path: "/etc/nftables.conf", Data: []byte("new")}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Changes) != 1 || !plan.WouldLoadNftables {
		t.Fatalf("unexpected plan: %#v", plan)
	}
	if len(r.calls) != 0 {
		t.Fatalf("dry-run executed commands: %#v", r.calls)
	}
	if _, err := os.Stat(filepath.Join(root, "etc/nftables.conf")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dry-run wrote file: %v", err)
	}
}

func TestApplyCanRequireRoot(t *testing.T) {
	_, _, err := Apply(context.Background(), Options{RequireRoot: true, EUID: func() int { return 1000 }})
	if err == nil {
		t.Fatal("expected root error")
	}
}

func TestApplyWritesBackupsLoadsAndSchedulesRollback(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "etc"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "etc/nftables.conf")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := &fakeRunner{}
	tx, _, err := Apply(context.Background(), Options{
		Root:            root,
		RunRoot:         "/run/cnftctl",
		Runner:          r,
		Systemd:         systemd.Manager{Runner: r},
		NftConfigPath:   "/etc/nftables.conf",
		RollbackTimeout: DefaultRollbackTimeout,
		Now:             func() time.Time { return time.Unix(100, 0).UTC() },
		Files:           []File{{Path: "/etc/nftables.conf", Data: []byte("new"), Mode: 0o600}},
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Fatalf("file = %q", data)
	}
	if tx.RollbackDeadline.Sub(tx.StartedAt) != DefaultRollbackTimeout {
		t.Fatalf("rollback timeout = %s", tx.RollbackDeadline.Sub(tx.StartedAt))
	}
	wantNames := []string{"nft", "nft", "systemd-run"}
	var gotNames []string
	for _, c := range r.calls {
		gotNames = append(gotNames, c.name)
	}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("commands = %#v, want %#v", r.calls, wantNames)
	}
	if _, err := os.Stat(tx.Files[0].BackupPath); err != nil {
		t.Fatalf("backup missing: %v", err)
	}
}

func TestConcurrentApplyRejected(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "run/cnftctl"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "run/cnftctl/apply.lock"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := Apply(context.Background(), Options{Root: root, Runner: &fakeRunner{}, NftConfigPath: "/etc/nftables.conf", Files: []File{{Path: "/etc/nftables.conf", Data: []byte("x")}}})
	if err == nil {
		t.Fatal("expected lock error")
	}
}

func TestRollbackRestoresPreviousFilesAndReloads(t *testing.T) {
	root := t.TempDir()
	r := &fakeRunner{}
	tx, _, err := Apply(context.Background(), Options{Root: root, Runner: r, Systemd: systemd.Manager{Runner: r}, NftConfigPath: "/etc/nftables.conf", Files: []File{{Path: "/etc/nftables.conf", Data: []byte("new")}}})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "etc/nftables.conf")
	if err := os.WriteFile(path, []byte("changed after apply"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Restore(context.Background(), root, filepath.Join(root, "run/cnftctl", tx.ID), r); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected rollback to remove newly-created file, got %v", err)
	}
}

func TestConfirmMarksTransactionAndCancelsUnit(t *testing.T) {
	root := t.TempDir()
	r := &fakeRunner{}
	tx, _, err := Apply(context.Background(), Options{Root: root, Runner: r, Systemd: systemd.Manager{Runner: r}, NftConfigPath: "/etc/nftables.conf", Files: []File{{Path: "/etc/nftables.conf", Data: []byte("new")}}})
	if err != nil {
		t.Fatal(err)
	}
	confirmed, err := Confirm(context.Background(), root, "/run/cnftctl", tx.ID, systemd.Manager{Runner: r})
	if err != nil {
		t.Fatal(err)
	}
	if !confirmed.Confirmed {
		t.Fatal("transaction was not confirmed")
	}
	last := r.calls[len(r.calls)-1]
	if last.name != "systemctl" || !reflect.DeepEqual(last.args, []string{"cancel", tx.UnitName}) {
		t.Fatalf("last call = %#v", last)
	}
}

func TestPendingReturnsOnlyUnresolvedTransactions(t *testing.T) {
	root := t.TempDir()
	r := &fakeRunner{}
	pendingTx, _, err := Apply(context.Background(), Options{Root: root, Runner: r, Systemd: systemd.Manager{Runner: r}, NftConfigPath: "/etc/nftables.conf", Files: []File{{Path: "/etc/nftables.conf", Data: []byte("pending")}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "run/cnftctl/apply.lock")); err != nil {
		t.Fatal(err)
	}
	confirmedTx, _, err := Apply(context.Background(), Options{Root: root, Runner: r, Systemd: systemd.Manager{Runner: r}, NftConfigPath: "/etc/nftables.conf", Files: []File{{Path: "/etc/nftables.conf", Data: []byte("confirmed")}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Confirm(context.Background(), root, "/run/cnftctl", confirmedTx.ID, systemd.Manager{Runner: r}); err != nil {
		t.Fatal(err)
	}

	pending, err := Pending(root, "/run/cnftctl")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].ID != pendingTx.ID {
		t.Fatalf("unexpected pending transactions: %#v", pending)
	}
}

func TestPendingMissingRunRootIsEmpty(t *testing.T) {
	pending, err := Pending(t.TempDir(), "/run/cnftctl")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected no pending transactions, got %#v", pending)
	}
}
