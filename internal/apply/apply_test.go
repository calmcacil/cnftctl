package apply

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
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
	calls      []call
	results    []nft.Result
	liveMarker string
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) nft.Result {
	f.calls = append(f.calls, call{name, append([]string(nil), args...)})
	if len(f.results) == 0 {
		if name == "nft" && len(args) >= 4 && args[0] == "list" && args[1] == "table" {
			return nft.Result{Stdout: `table inet hostfw { comment "` + f.liveMarker + `" }`}
		}
		if name == "nft" && len(args) == 2 && args[0] == "-f" {
			if data, err := os.ReadFile(args[1]); err == nil {
				if marker := generationMarker.Find(data); marker != nil {
					f.liveMarker = string(marker)
				}
			}
		}
		return nft.Result{}
	}
	r := f.results[0]
	f.results = f.results[1:]
	return r
}
func policy(s string) []File {
	return []File{{Path: "/desired/firewall.nft", Data: []byte(s)}, {Path: "/desired/open-ports.nft", Data: []byte("set open_ports {}\n")}, {Path: "/desired/whitelist.nft", Data: []byte("define whitelist_v4 = {}\n")}}
}

func tempRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Cleanup(func() {
		_ = filepath.Walk(filepath.Join(root, "var/lib/cnftctl/generations"), func(path string, info os.FileInfo, err error) error {
			if err == nil && info.IsDir() {
				_ = os.Chmod(path, 0o700)
			}
			return nil
		})
	})
	return root
}

func TestManifestDeterministic(t *testing.T) {
	a, _, _ := buildManifest(policy("a"))
	reversed := policy("a")
	reversed[0], reversed[2] = reversed[2], reversed[0]
	b, _, _ := buildManifest(reversed)
	ah, _ := manifestHash(a)
	bh, _ := manifestHash(b)
	if ah != bh {
		t.Fatalf("hashes differ: %s %s", ah, bh)
	}
}
func TestDDNSIntentChangesGenerationIdentity(t *testing.T) {
	a, err := PlanFilesWithDDNS("", policy("a"), false)
	if err != nil {
		t.Fatal(err)
	}
	b, err := PlanFilesWithDDNS("", policy("a"), true)
	if err != nil {
		t.Fatal(err)
	}
	if a.Generation == b.Generation {
		t.Fatal("DDNS intent was omitted from generation identity")
	}
}
func TestSemanticGenerationIgnoresMaterializedGenerationPath(t *testing.T) {
	logical := policy(`include "/var/lib/cnftctl/generations/{generation}/whitelist.nft"`)
	plan, err := PlanFiles("", logical, "firewall.nft")
	if err != nil {
		t.Fatal(err)
	}
	materialized := policy(`include "/var/lib/cnftctl/generations/` + plan.Generation + `/whitelist.nft"`)
	final, err := PlanFiles("", materialized, "firewall.nft")
	if err != nil {
		t.Fatal(err)
	}
	if final.Generation != plan.Generation {
		t.Fatalf("logical generation %s, materialized generation %s", plan.Generation, final.Generation)
	}
}
func TestSemanticGenerationIgnoresMaterializedGenerationMarker(t *testing.T) {
	logical := policy(`table inet hostfw { comment "` + OwnershipMarker + `:generation:{generation}" }`)
	plan, err := PlanFiles("", logical, "firewall.nft")
	if err != nil {
		t.Fatal(err)
	}
	materialized := policy(`table inet hostfw { comment "` + OwnershipMarker + `:generation:` + plan.Generation + `" }`)
	final, err := PlanFiles("", materialized, "firewall.nft")
	if err != nil || final.Generation != plan.Generation {
		t.Fatalf("materialized generation=%q err=%v, want %q", final.Generation, err, plan.Generation)
	}
}
func TestDryRunDoesNotWriteOrRun(t *testing.T) {
	root := tempRoot(t)
	r := &fakeRunner{}
	_, p, err := Apply(context.Background(), Options{Root: root, DryRun: true, Runner: r, Files: policy("x")})
	if err != nil {
		t.Fatal(err)
	}
	if p.Generation == "" || !p.WouldLoadNftables {
		t.Fatalf("plan = %#v", p)
	}
	if len(r.calls) != 0 {
		t.Fatalf("calls = %#v", r.calls)
	}
	if _, err := os.Stat(filepath.Join(root, "var")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dry run wrote state: %v", err)
	}
}
func TestApplyArmsAndVerifiesBeforeActivation(t *testing.T) {
	root := tempRoot(t)
	r := &fakeRunner{results: []nft.Result{{Err: errors.New("missing"), Stderr: "No such table"}, {}, {}, {}}}
	tx, _, err := Apply(context.Background(), Options{Root: root, Runner: r, Systemd: systemd.Manager{Runner: r}, Files: policy("table inet hostfw { comment \"" + OwnershipMarker + "\" }\n")})
	if err != nil {
		t.Fatal(err)
	}
	if tx.Phase != PhaseActivated {
		t.Fatalf("phase = %s", tx.Phase)
	}
	var names []string
	for _, c := range r.calls {
		names = append(names, c.name+" "+strings.Join(c.args, " "))
	}
	joined := strings.Join(names, "\n")
	arm := strings.Index(joined, "systemctl start")
	activate := strings.Index(joined, "systemctl restart "+systemd.FirewallService)
	if arm < 0 || activate < 0 || arm > activate {
		t.Fatalf("ordering:\n%s", joined)
	}
	if !strings.Contains(r.calls[1].args[len(r.calls[1].args)-1], "cnftctl-candidate-") {
		t.Fatalf("validation path = %#v", r.calls[1])
	}
	if _, err := os.Stat(filepath.Join(root, "etc/nftables.conf")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("touched /etc/nftables.conf: %v", err)
	}
}
func TestGenerationManifestHashesExactMaterializedBytes(t *testing.T) {
	root := tempRoot(t)
	r := &fakeRunner{results: []nft.Result{{Err: errors.New("missing"), Stderr: "No such table"}, {}, {}, {}}}
	files := policy(`include "/var/lib/cnftctl/generations/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/whitelist.nft"`)
	tx, _, err := Apply(context.Background(), Options{Root: root, Runner: r, Systemd: systemd.Manager{Runner: r}, Files: files})
	if err != nil {
		t.Fatal(err)
	}
	manifestBytes, err := os.ReadFile(rooted(root, GenerationRoot+"/"+tx.Generation+"/manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	_, finalized, err := FinalizeFiles(files, false)
	if err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256(finalized[0].Data)
	for _, entry := range manifest.Files {
		if entry.Path == "firewall.nft" && entry.SHA256 != hex.EncodeToString(want[:]) {
			t.Fatalf("manifest hash = %s, want exact-byte hash %x", entry.SHA256, want)
		}
	}
}

func TestSSHOverrideAuditPersists(t *testing.T) {
	root := tempRoot(t)
	r := &fakeRunner{results: []nft.Result{{Err: errors.New("missing"), Stderr: "No such table"}, {}, {}, {}}}
	tx, _, err := Apply(context.Background(), Options{Root: root, Runner: r, Systemd: systemd.Manager{Runner: r}, Files: policy("x"), SSHOverrideAcknowledged: true, SSHOverrideReason: " maintenance window ", SSHOverrideSource: "cli", SSHOverrideContext: "SSH_CONNECTION"})
	if err != nil {
		t.Fatal(err)
	}
	disk, err := readTransaction(rooted(root, TransactionRoot+"/"+tx.ID))
	if err != nil {
		t.Fatal(err)
	}
	if disk.SSHOverride == nil || disk.SSHOverride.Reason != "maintenance window" || !disk.SSHOverride.Acknowledged {
		t.Fatalf("audit = %#v", disk.SSHOverride)
	}
}

func TestSSHOverrideRequiresReason(t *testing.T) {
	_, _, err := Apply(context.Background(), Options{DryRun: true, Files: policy("x"), SSHOverrideAcknowledged: true})
	if err == nil || !strings.Contains(err.Error(), "reason is required") {
		t.Fatalf("error = %v", err)
	}
}

func TestRollbackTimeoutMatchesInstalledStaticTimer(t *testing.T) {
	if DefaultRollbackTimeout != 2*time.Minute {
		t.Fatalf("default timeout = %s", DefaultRollbackTimeout)
	}
	_, _, err := Apply(context.Background(), Options{DryRun: true, Files: policy("x"), RollbackTimeout: time.Minute})
	if err == nil || !strings.Contains(err.Error(), "must be 2m0s") {
		t.Fatalf("error = %v", err)
	}
}
func TestUnknownExistingTableRejected(t *testing.T) {
	r := &fakeRunner{results: []nft.Result{{Stdout: "table inet hostfw {}"}}}
	_, _, err := Apply(context.Background(), Options{Root: t.TempDir(), Runner: r, Files: policy("x")})
	if err == nil || !strings.Contains(err.Error(), "not owned") {
		t.Fatalf("error = %v", err)
	}
	if len(r.calls) != 1 {
		t.Fatalf("commands = %#v", r.calls)
	}
}
func TestNoopApplyRunsNoActivation(t *testing.T) {
	root := tempRoot(t)
	r := &fakeRunner{results: []nft.Result{{Err: errors.New("missing"), Stderr: "No such table"}, {}, {}, {}}}
	opts := Options{Root: root, Runner: r, Systemd: systemd.Manager{Runner: r}, Files: policy("x")}
	tx, _, err := Apply(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	r.liveMarker = OwnershipMarker + ":generation:" + tx.Generation
	if _, err = Confirm(context.Background(), root, "", tx.ID, systemd.Manager{Runner: r}); err != nil {
		t.Fatal(err)
	}
	before := len(r.calls)
	tx, plan, err := Apply(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if tx.Phase != PhaseConfirmed || len(plan.Changes) != 0 || strings.Contains(fmtCalls(r.calls[before:]), "restart "+systemd.FirewallService) {
		t.Fatalf("not a no-op: tx=%#v plan=%#v calls=%#v", tx, plan, r.calls[before:])
	}
}

func TestSameGenerationMissingLiveTableCreatesFreshRepair(t *testing.T) {
	root := tempRoot(t)
	r := &fakeRunner{results: []nft.Result{{Err: errors.New("missing"), Stderr: "No such table"}, {}, {}, {}}}
	opts := Options{Root: root, Runner: r, Systemd: systemd.Manager{Runner: r}, Files: policy("x")}
	firstTx, _, err := Apply(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	r.liveMarker = OwnershipMarker + ":generation:" + firstTx.Generation
	if _, err := Confirm(context.Background(), root, "", firstTx.ID, systemd.Manager{Runner: r}); err != nil {
		t.Fatal(err)
	}
	r.results = []nft.Result{{Err: errors.New("missing"), Stderr: "No such table"}, {}, {}, {Err: errors.New("missing"), Stderr: "No such table"}, {}, {}, {}, {}, {}}
	repair, _, err := Apply(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if repair.ID == "" || !repair.FreshInstall || repair.PreviousGeneration != "" || repair.Phase != PhaseActivated {
		t.Fatalf("unsafe repair transaction: %#v", repair)
	}
}

func TestSameGenerationReplacedMarkerFailsClosed(t *testing.T) {
	root := tempRoot(t)
	r := &fakeRunner{results: []nft.Result{{Err: errors.New("missing"), Stderr: "No such table"}, {}, {}, {}}}
	opts := Options{Root: root, Runner: r, Systemd: systemd.Manager{Runner: r}, Files: policy("x")}
	tx, _, err := Apply(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	r.liveMarker = OwnershipMarker + ":generation:" + strings.Repeat("f", 64)
	if _, err := Confirm(context.Background(), root, "", tx.ID, systemd.Manager{Runner: r}); err == nil {
		t.Fatal("confirmed replaced live table")
	}
	// Make the fixture terminal so the next apply reaches same-generation verification.
	disk, _ := readTransaction(rooted(root, TransactionRoot+"/"+tx.ID))
	disk.Confirmed, disk.Phase = true, PhaseConfirmed
	if err := writeTransaction(rooted(root, TransactionRoot+"/"+tx.ID), disk); err != nil {
		t.Fatal(err)
	}
	before := len(r.calls)
	if _, _, err := Apply(context.Background(), opts); err == nil || !strings.Contains(err.Error(), "exact generation marker") {
		t.Fatalf("replaced marker error = %v", err)
	}
	if strings.Contains(fmtCalls(r.calls[before:]), "restart "+systemd.FirewallService) {
		t.Fatal("replaced table was overwritten")
	}
}

func TestSameGenerationTimerDriftReconcilesWithoutFirewallReload(t *testing.T) {
	root := tempRoot(t)
	r := &fakeRunner{results: []nft.Result{{Err: errors.New("missing"), Stderr: "No such table"}, {}, {}, {}}}
	opts := Options{Root: root, Runner: r, Systemd: systemd.Manager{Runner: r}, Files: policy("x"), DDNSDesired: true}
	tx, _, err := Apply(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	r.liveMarker = OwnershipMarker + ":generation:" + tx.Generation
	if _, err := Confirm(context.Background(), root, "", tx.ID, systemd.Manager{Runner: r}); err != nil {
		t.Fatal(err)
	}
	r.results = []nft.Result{{Stdout: `table inet hostfw { comment "` + r.liveMarker + `" }`}, {Err: errors.New("inactive")}, {Err: errors.New("disabled")}, {}}
	before := len(r.calls)
	got, _, err := Apply(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	calls := fmtCalls(r.calls[before:])
	if !got.Confirmed || !strings.Contains(calls, "enable --now "+systemd.DDNSRefreshTimer) || strings.Contains(calls, "restart "+systemd.FirewallService) {
		t.Fatalf("timer reconciliation calls:\n%s", calls)
	}
}
func TestConfirmPersistsBeforeTimerStop(t *testing.T) {
	root := t.TempDir()
	id := "0123456789abcdef0123456789abcdef"
	generation := strings.Repeat("a", 64)
	tx := Transaction{ID: id, Phase: PhaseActivated, Generation: generation}
	dir := rooted(root, TransactionRoot+"/"+id)
	if err := writeTransaction(dir, tx); err != nil {
		t.Fatal(err)
	}
	if err := setActive(root, generation); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(rooted(root, OwnershipPath), ownership{Marker: OwnershipMarker, Generation: generation}, 0o600); err != nil {
		t.Fatal(err)
	}
	r := &fakeRunner{results: []nft.Result{{Stdout: `table inet hostfw { comment "` + OwnershipMarker + `:generation:` + generation + `" }`}, {Err: errors.New("stop failed")}}}
	got, err := Confirm(context.Background(), root, "", id, systemd.Manager{Runner: r})
	if err == nil {
		t.Fatal("expected stop failure")
	}
	if !got.Confirmed {
		t.Fatal("return state not confirmed")
	}
	disk, err := readTransaction(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !disk.Confirmed || disk.Phase != PhaseConfirmed {
		t.Fatalf("disk state = %#v", disk)
	}
}
func TestFreshRollbackDeletesOnlyManagedTable(t *testing.T) {
	root := t.TempDir()
	id := "0123456789abcdef0123456789abcdef"
	generation := strings.Repeat("a", 64)
	tx := Transaction{ID: id, Phase: PhaseActivated, FreshInstall: true, Generation: generation}
	dir := rooted(root, TransactionRoot+"/"+id)
	if err := writeTransaction(dir, tx); err != nil {
		t.Fatal(err)
	}
	if err := setActive(root, generation); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(rooted(root, OwnershipPath), ownership{Marker: OwnershipMarker, Generation: generation}, 0o600); err != nil {
		t.Fatal(err)
	}
	r := &fakeRunner{liveMarker: OwnershipMarker + ":generation:" + generation}
	if err := Restore(context.Background(), root, dir, r); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if len(r.calls) < 3 || r.calls[0].name != "nft" || r.calls[1].name != "nft" || r.calls[1].args[0] != "-f" {
		t.Fatalf("calls = %#v", r.calls)
	}
}

func TestValidateCandidateDoesNotWriteDurableState(t *testing.T) {
	root := tempRoot(t)
	r := &fakeRunner{}
	generation, err := ValidateCandidate(context.Background(), Options{Root: root, Runner: r, Files: policy("x")})
	if err != nil || generation == "" {
		t.Fatalf("generation=%q err=%v", generation, err)
	}
	if _, err := os.Stat(filepath.Join(root, "var")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("validator wrote durable state: %v", err)
	}
	if len(r.calls) != 1 || r.calls[0].args[0] != "-c" {
		t.Fatalf("calls = %#v", r.calls)
	}
}

func TestApplyTimerFailureRollsBackSynchronously(t *testing.T) {
	root := tempRoot(t)
	r := &fakeRunner{results: []nft.Result{{Err: errors.New("missing"), Stderr: "No such table"}, {}, {}, {}, {Err: errors.New("timer failed")}, {}, {}, {}}}
	tx, _, err := Apply(context.Background(), Options{Root: root, Runner: r, Systemd: systemd.Manager{Runner: r}, Files: policy("x"), DDNSDesired: true})
	if err == nil || !strings.Contains(err.Error(), "timer failed") {
		t.Fatalf("error = %v", err)
	}
	disk, readErr := readTransaction(rooted(root, TransactionRoot+"/"+tx.ID))
	if readErr != nil || !disk.RolledBack {
		t.Fatalf("transaction=%#v readErr=%v", disk, readErr)
	}
}
func TestReconcileRollsBackEveryUnconfirmedTransaction(t *testing.T) {
	root := t.TempDir()
	for _, id := range []string{"0123456789abcdef0123456789abcdef", "abcdef0123456789abcdef0123456789"} {
		if err := writeTransaction(rooted(root, TransactionRoot+"/"+id), Transaction{ID: id, FreshInstall: true, Phase: PhasePrepared}); err != nil {
			t.Fatal(err)
		}
	}
	r := &fakeRunner{}
	if err := Reconcile(context.Background(), root, r); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	pending, err := Pending(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending = %#v", pending)
	}
}
func TestRestoreRejectsArbitraryDirectory(t *testing.T) {
	if err := Restore(context.Background(), t.TempDir(), "/tmp/evil", &fakeRunner{}); err == nil {
		t.Fatal("expected confinement error")
	}
}

func TestApplyCommandFaultBoundariesNeverLeaveUnarmedActivePolicy(t *testing.T) {
	tests := []struct {
		name string
		fail int
	}{
		{"ownership inspection", 1},
		{"exact candidate validation", 2},
		{"arm rollback timer", 3},
		{"verify rollback timer", 4},
		{"activate firewall", 5},
		{"reconcile DDNS timer", 6},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := tempRoot(t)
			r := &nthFailureRunner{fail: tc.fail}
			tx, _, err := Apply(context.Background(), Options{Root: root, Runner: r, Systemd: systemd.Manager{Runner: r}, Files: policy("table inet hostfw { comment \"" + OwnershipMarker + "\" }\n")})
			if err == nil {
				t.Fatal("fault was not propagated")
			}
			active, activeErr := activeGeneration(root)
			if activeErr != nil {
				t.Fatal(activeErr)
			}
			if tc.fail <= 4 && active != "" {
				t.Fatalf("policy became active before rollback was armed: %s", active)
			}
			if tc.fail >= 5 && tx.ID != "" {
				disk, readErr := readTransaction(rooted(root, TransactionRoot+"/"+tx.ID))
				if readErr != nil || !disk.RolledBack || active != "" {
					t.Fatalf("failed activation was not synchronously recovered: tx=%#v active=%q err=%v", disk, active, readErr)
				}
			}
		})
	}
}

type nthFailureRunner struct {
	mu   sync.Mutex
	n    int
	fail int
}

func (r *nthFailureRunner) Run(_ context.Context, _ string, _ ...string) nft.Result {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.n++
	if r.n == r.fail {
		return nft.Result{Err: errors.New("injected boundary failure"), Stderr: "injected"}
	}
	if r.n == 1 {
		return nft.Result{Err: errors.New("missing"), Stderr: "No such table"}
	}
	return nft.Result{}
}

func TestConfirmRetryAndRollbackAreIdempotent(t *testing.T) {
	root := tempRoot(t)
	id := "0123456789abcdef0123456789abcdef"
	dir := rooted(root, TransactionRoot+"/"+id)
	generation := strings.Repeat("a", 64)
	if err := writeTransaction(dir, Transaction{ID: id, Phase: PhaseActivated, FreshInstall: true, Generation: generation}); err != nil {
		t.Fatal(err)
	}
	if err := setActive(root, generation); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(rooted(root, OwnershipPath), ownership{Marker: OwnershipMarker, Generation: generation}, 0o600); err != nil {
		t.Fatal(err)
	}
	r := &fakeRunner{liveMarker: OwnershipMarker + ":generation:" + generation}
	for i := 0; i < 2; i++ {
		if _, err := Confirm(context.Background(), root, "", id, systemd.Manager{Runner: r}); err != nil {
			t.Fatalf("confirm %d: %v", i, err)
		}
	}
	before := len(r.calls)
	for i := 0; i < 2; i++ {
		if err := Restore(context.Background(), root, dir, r); err != nil {
			t.Fatalf("rollback after confirmation %d: %v", i, err)
		}
	}
	if len(r.calls) != before {
		t.Fatalf("rollback raced past durable confirmation: %#v", r.calls[before:])
	}
}

func TestRollbackRestoresPriorGenerationAndDDNSIntent(t *testing.T) {
	root := tempRoot(t)
	previous := strings.Repeat("a", 64)
	generation := strings.Repeat("b", 64)
	prevDir := rooted(root, GenerationRoot+"/"+previous)
	if err := os.MkdirAll(prevDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(prevDir, "firewall.nft"), []byte(`table inet hostfw { comment "`+OwnershipMarker+`:generation:`+previous+`" }`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(prevDir, "manifest.json"), Manifest{Version: 1, DDNSDesired: true}, 0o600); err != nil {
		t.Fatal(err)
	}
	id := "0123456789abcdef0123456789abcdef"
	dir := rooted(root, TransactionRoot+"/"+id)
	if err := writeTransaction(dir, Transaction{ID: id, Phase: PhaseActivated, Generation: generation, PreviousGeneration: previous}); err != nil {
		t.Fatal(err)
	}
	if err := setActive(root, generation); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(rooted(root, OwnershipPath), ownership{Marker: OwnershipMarker, Generation: generation}, 0o600); err != nil {
		t.Fatal(err)
	}
	r := &fakeRunner{liveMarker: OwnershipMarker + ":generation:" + generation}
	if err := Restore(context.Background(), root, dir, r); err != nil {
		t.Fatal(err)
	}
	active, err := activeGeneration(root)
	if err != nil || active != previous {
		t.Fatalf("active=%q err=%v", active, err)
	}
	joined := fmtCalls(r.calls)
	if !strings.Contains(joined, filepath.Join(prevDir, "firewall.nft")) || !strings.Contains(joined, "enable --now "+systemd.DDNSRefreshTimer) {
		t.Fatalf("rollback commands:\n%s", joined)
	}
}

func TestUpdateRollbackRefusesForeignRuntimeState(t *testing.T) {
	for _, field := range []string{"selector", "ownership", "live marker"} {
		t.Run(field, func(t *testing.T) {
			root, dir, tx, r := updateRollbackFixture(t)
			foreign := strings.Repeat("c", 64)
			switch field {
			case "selector":
				if err := setActive(root, foreign); err != nil {
					t.Fatal(err)
				}
			case "ownership":
				if err := writeJSON(rooted(root, OwnershipPath), ownership{Marker: OwnershipMarker, Generation: foreign}, 0o600); err != nil {
					t.Fatal(err)
				}
			case "live marker":
				r.liveMarker = OwnershipMarker + ":generation:" + foreign
			}
			before := len(r.calls)
			if err := Restore(context.Background(), root, dir, r); err == nil {
				t.Fatal("accepted foreign runtime state")
			}
			disk, err := readTransaction(dir)
			if err != nil || disk.RolledBack || disk.Phase != PhaseActivated {
				t.Fatalf("transaction=%#v err=%v", disk, err)
			}
			for _, call := range r.calls[before:] {
				if call.name != "nft" || len(call.args) == 0 || call.args[0] != "list" {
					t.Fatalf("rollback mutated runtime or timer: %#v (tx=%#v)", r.calls[before:], tx)
				}
			}
		})
	}
}

func TestUpdateRollbackResumesEveryRestorationBoundary(t *testing.T) {
	for stage := 0; stage <= 3; stage++ {
		t.Run(fmt.Sprintf("stage %d", stage), func(t *testing.T) {
			root, dir, tx, r := updateRollbackFixture(t)
			if stage >= 1 {
				if err := setActive(root, tx.PreviousGeneration); err != nil {
					t.Fatal(err)
				}
			}
			if stage >= 2 {
				r.liveMarker = OwnershipMarker + ":generation:" + tx.PreviousGeneration
			}
			if stage >= 3 {
				if err := writeJSON(rooted(root, OwnershipPath), ownership{Marker: OwnershipMarker, Generation: tx.PreviousGeneration}, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := Restore(context.Background(), root, dir, r); err != nil {
				t.Fatal(err)
			}
			disk, err := readTransaction(dir)
			if err != nil || !disk.RolledBack {
				t.Fatalf("transaction=%#v err=%v", disk, err)
			}
			active, _ := activeGeneration(root)
			var own ownership
			if err := readJSON(rooted(root, OwnershipPath), &own); err != nil {
				t.Fatal(err)
			}
			if active != tx.PreviousGeneration || own.Generation != tx.PreviousGeneration || r.liveMarker != OwnershipMarker+":generation:"+tx.PreviousGeneration {
				t.Fatalf("incomplete restoration: active=%q ownership=%#v live=%q", active, own, r.liveMarker)
			}
		})
	}
}

func updateRollbackFixture(t *testing.T) (string, string, Transaction, *fakeRunner) {
	t.Helper()
	root := tempRoot(t)
	tx := Transaction{ID: "0123456789abcdef0123456789abcdef", Phase: PhaseActivated, Generation: strings.Repeat("a", 64), PreviousGeneration: strings.Repeat("b", 64)}
	dir := rooted(root, TransactionRoot+"/"+tx.ID)
	prevDir := rooted(root, GenerationRoot+"/"+tx.PreviousGeneration)
	if err := os.MkdirAll(prevDir, 0o700); err != nil {
		t.Fatal(err)
	}
	policy := []byte(`table inet hostfw { comment "` + OwnershipMarker + `:generation:` + tx.PreviousGeneration + `" }`)
	if err := os.WriteFile(filepath.Join(prevDir, "firewall.nft"), policy, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(prevDir, "manifest.json"), Manifest{Version: 1}, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeTransaction(dir, tx); err != nil {
		t.Fatal(err)
	}
	if err := setActive(root, tx.Generation); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(rooted(root, OwnershipPath), ownership{Marker: OwnershipMarker, Generation: tx.Generation}, 0o600); err != nil {
		t.Fatal(err)
	}
	return root, dir, tx, &fakeRunner{liveMarker: OwnershipMarker + ":generation:" + tx.Generation}
}

func TestReconcileAfterRebootHonorsConfirmedAndRollsBackPending(t *testing.T) {
	root := tempRoot(t)
	confirmed := "0123456789abcdef0123456789abcdef"
	pending := "abcdef0123456789abcdef0123456789"
	if err := writeTransaction(rooted(root, TransactionRoot+"/"+confirmed), Transaction{ID: confirmed, Confirmed: true, Phase: PhaseConfirmed}); err != nil {
		t.Fatal(err)
	}
	if err := writeTransaction(rooted(root, TransactionRoot+"/"+pending), Transaction{ID: pending, FreshInstall: true, Phase: PhasePrepared}); err != nil {
		t.Fatal(err)
	}
	r := &fakeRunner{}
	if err := Reconcile(context.Background(), root, r); err != nil {
		t.Fatal(err)
	}
	if len(r.calls) != 0 {
		t.Fatalf("reconcile commands = %#v", r.calls)
	}
	disk, err := readTransaction(rooted(root, TransactionRoot+"/"+pending))
	if err != nil || !disk.RolledBack {
		t.Fatalf("pending tx=%#v err=%v", disk, err)
	}
}

func TestSharedFirewallLockContention(t *testing.T) {
	root := tempRoot(t)
	path := rooted(root, RunRoot)
	lock, err := lockRuntime(path)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := WithFirewallLock(root, func() error { t.Fatal("contended callback ran"); return nil }); err == nil || !strings.Contains(err.Error(), "another cnftctl operation") {
		t.Fatalf("contention error = %v", err)
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}
}

func TestTransactionAndStateConfinement(t *testing.T) {
	root := tempRoot(t)
	for _, id := range []string{"../escape", strings.Repeat("g", 32), strings.Repeat("a", 31)} {
		if _, err := Confirm(context.Background(), root, "", id, systemd.Manager{Runner: &fakeRunner{}}); err == nil {
			t.Fatalf("accepted transaction id %q", id)
		}
	}
	id := "0123456789abcdef0123456789abcdef"
	dir := rooted(root, TransactionRoot+"/"+id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "outside.json")
	if err := os.WriteFile(target, []byte(`{"id":"`+id+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "state.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := Confirm(context.Background(), root, "", id, systemd.Manager{Runner: &fakeRunner{}}); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlink state error = %v", err)
	}
}

func TestCorruptTransactionsAndGenerationsFailClosed(t *testing.T) {
	root := tempRoot(t)
	id := "0123456789abcdef0123456789abcdef"
	dir := rooted(root, TransactionRoot+"/"+id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Confirm(context.Background(), root, "", id, systemd.Manager{Runner: &fakeRunner{}}); err == nil {
		t.Fatal("accepted corrupt transaction")
	}

	gen := strings.Repeat("b", 64)
	genDir := rooted(root, GenerationRoot+"/"+gen)
	if err := os.MkdirAll(genDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(genDir, "firewall.nft"), []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	m, _, err := buildManifest([]File{{Path: "firewall.nft", Data: []byte("expected")}})
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyGeneration(genDir, m); err == nil {
		t.Fatalf("manifest error = %v", err)
	}
}

func TestGenerationReuseRejectsManifestModeAndExtraFileDrift(t *testing.T) {
	root := tempRoot(t)
	m, files, err := buildManifest(policy("expected"))
	if err != nil {
		t.Fatal(err)
	}
	m.DDNSDesired = true
	semantic, _, _ := buildSemanticManifest(files)
	semantic.DDNSDesired = true
	generation, _ := manifestHash(semantic)
	dir := rooted(root, GenerationRoot+"/"+generation)
	if err := writeGeneration(dir, m, files); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "extra"), []byte("foreign"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	if err := writeGeneration(dir, m, files); err == nil {
		t.Fatal("reused generation with an unexpected file")
	}
}

func TestPreactivationFreshRollbackNeverTouchesLiveTable(t *testing.T) {
	root := tempRoot(t)
	id := "0123456789abcdef0123456789abcdef"
	dir := rooted(root, TransactionRoot+"/"+id)
	if err := writeTransaction(dir, Transaction{ID: id, Phase: PhasePrepared, FreshInstall: true, Generation: strings.Repeat("a", 64)}); err != nil {
		t.Fatal(err)
	}
	r := &fakeRunner{}
	if err := Restore(context.Background(), root, dir, r); err != nil {
		t.Fatal(err)
	}
	if len(r.calls) != 0 {
		t.Fatalf("preactivation rollback touched runtime: %#v", r.calls)
	}
}

func TestActivatingFreshRollbackUsesLiveMarkerWithPartialState(t *testing.T) {
	root := tempRoot(t)
	id := "0123456789abcdef0123456789abcdef"
	generation := strings.Repeat("a", 64)
	dir := rooted(root, TransactionRoot+"/"+id)
	if err := writeTransaction(dir, Transaction{ID: id, Phase: PhaseActivating, FreshInstall: true, Generation: generation}); err != nil {
		t.Fatal(err)
	}
	if err := setActive(root, generation); err != nil {
		t.Fatal(err)
	}
	r := &fakeRunner{liveMarker: OwnershipMarker + ":generation:" + generation}
	if err := Restore(context.Background(), root, dir, r); err != nil {
		t.Fatal(err)
	}
	disk, err := readTransaction(dir)
	if err != nil || !disk.RolledBack {
		t.Fatalf("transaction=%#v err=%v", disk, err)
	}
}

func TestActivatingFreshRollbackRefusesWrongGenerationTable(t *testing.T) {
	root := tempRoot(t)
	id := "0123456789abcdef0123456789abcdef"
	generation := strings.Repeat("a", 64)
	dir := rooted(root, TransactionRoot+"/"+id)
	if err := writeTransaction(dir, Transaction{ID: id, Phase: PhaseActivating, FreshInstall: true, Generation: generation}); err != nil {
		t.Fatal(err)
	}
	r := &fakeRunner{liveMarker: OwnershipMarker + ":generation:" + strings.Repeat("b", 64)}
	if err := Restore(context.Background(), root, dir, r); err == nil {
		t.Fatal("deleted or accepted a table owned by another generation")
	}
	disk, _ := readTransaction(dir)
	if disk.RolledBack {
		t.Fatal("incomplete rollback became terminal")
	}
}

func TestVerifyInstalledAssetsStrictInventory(t *testing.T) {
	root := tempRoot(t)
	files := map[string][]byte{
		"/usr/bin/cnftctl":                                     []byte("binary"),
		"/usr/lib/cnftctl/cnftctl-recover":                     []byte("recover"),
		"/usr/lib/systemd/system/cnftctl-firewall.service":     []byte("ExecStart=/usr/bin/nft -f /var/lib/cnftctl/active/firewall.nft\n"),
		"/usr/lib/systemd/system/cnftctl-reconcile.service":    []byte("ExecStart=/usr/bin/cnftctl reconcile\n"),
		"/usr/lib/systemd/system/cnftctl-rollback@.service":    []byte("ExecStart=/usr/bin/cnftctl rollback %i\n"),
		"/usr/lib/systemd/system/cnftctl-rollback@.timer":      []byte("timer"),
		"/usr/lib/systemd/system/cnftctl-ddns-refresh.service": []byte("service"),
		"/usr/lib/systemd/system/cnftctl-ddns-refresh.timer":   []byte("timer"),
		"/var/lib/cnftctl/delivery/manifest":                   []byte("format=1\nproduct=cnftctl\n"),
	}
	mapping := map[string]string{"/usr/bin/cnftctl": "bin/cnftctl", "/usr/lib/cnftctl/cnftctl-recover": "scripts/cnftctl-recover", "/var/lib/cnftctl/delivery/manifest": "manifest"}
	var sums []string
	for path, data := range files {
		full := rooted(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		mode := os.FileMode(0o644)
		if path == "/usr/bin/cnftctl" {
			mode = 0o755
		}
		if err := os.WriteFile(full, data, mode); err != nil {
			t.Fatal(err)
		}
		bundlePath := mapping[path]
		if bundlePath == "" {
			bundlePath = "systemd/" + filepath.Base(path)
		}
		sum := sha256.Sum256(data)
		sums = append(sums, hex.EncodeToString(sum[:])+"  "+bundlePath)
	}
	sort.Strings(sums)
	checksum := rooted(root, "/var/lib/cnftctl/delivery/SHA256SUMS")
	if err := os.WriteFile(checksum, []byte(strings.Join(sums, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyInstalledAssetsAt(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(checksum, []byte(sums[0]+"\n"+sums[0]+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyInstalledAssetsAt(root); err == nil {
		t.Fatal("accepted duplicate/incomplete inventory")
	}
}

func TestConfirmRejectsActivatedTransactionWithMismatchedSelector(t *testing.T) {
	root := tempRoot(t)
	id := "0123456789abcdef0123456789abcdef"
	tx := Transaction{ID: id, Phase: PhaseActivated, Generation: strings.Repeat("a", 64)}
	if err := writeTransaction(rooted(root, TransactionRoot+"/"+id), tx); err != nil {
		t.Fatal(err)
	}
	if err := setActive(root, strings.Repeat("b", 64)); err != nil {
		t.Fatal(err)
	}
	if _, err := Confirm(context.Background(), root, "", id, systemd.Manager{Runner: &fakeRunner{}}); err == nil {
		t.Fatal("confirmed mismatched active selector")
	}
}

func fmtCalls(calls []call) string {
	var lines []string
	for _, c := range calls {
		lines = append(lines, c.name+" "+strings.Join(c.args, " "))
	}
	return strings.Join(lines, "\n")
}
