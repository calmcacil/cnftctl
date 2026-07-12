package systemd

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/calmcacil/cnftctl/internal/nft"
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
	f.calls = append(f.calls, call{name, append([]string(nil), args...)})
	if len(f.results) == 0 {
		return nft.Result{}
	}
	r := f.results[0]
	f.results = f.results[1:]
	return r
}

func TestArmRollbackStartsAndVerifiesStaticTimer(t *testing.T) {
	r := &fakeRunner{}
	id := "0123456789abcdef0123456789abcdef"
	if err := (Manager{Runner: r}).ArmRollback(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	want := []call{{"systemctl", []string{"start", RollbackPrefix + id + ".timer"}}, {"systemctl", []string{"is-active", "--quiet", RollbackPrefix + id + ".timer"}}}
	if !reflect.DeepEqual(r.calls, want) {
		t.Fatalf("calls = %#v, want %#v", r.calls, want)
	}
}
func TestArmRollbackRejectsInactiveTimer(t *testing.T) {
	r := &fakeRunner{results: []nft.Result{{}, {Err: errors.New("inactive")}}}
	if err := (Manager{Runner: r}).ArmRollback(context.Background(), "0123456789abcdef0123456789abcdef"); err == nil {
		t.Fatal("expected inactive error")
	}
}
func TestCancelUsesStop(t *testing.T) {
	r := &fakeRunner{}
	id := "0123456789abcdef0123456789abcdef"
	if err := (Manager{Runner: r}).CancelRollback(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if r.calls[0].args[0] != "stop" {
		t.Fatalf("call = %#v", r.calls[0])
	}
}
func TestRejectsArbitraryInstance(t *testing.T) {
	if _, err := RollbackTimer("../../etc"); err == nil {
		t.Fatal("expected invalid ID")
	}
}

func TestReconcileDDNSTimerEnableDisableAndState(t *testing.T) {
	r := &fakeRunner{}
	m := Manager{Runner: r}
	if err := m.ReconcileDDNSTimer(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if err := m.ReconcileDDNSTimer(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	want := []call{{"systemctl", []string{"enable", "--now", DDNSRefreshTimer}}, {"systemctl", []string{"disable", "--now", DDNSRefreshTimer}}}
	if !reflect.DeepEqual(r.calls, want) {
		t.Fatalf("calls = %#v, want %#v", r.calls, want)
	}
	r = &fakeRunner{}
	state, err := (Manager{Runner: r}).DDNSState(context.Background())
	if err != nil || !state.Active || !state.Enabled {
		t.Fatalf("state=%#v err=%v", state, err)
	}
}

func TestFirewallEnablementIsExplicitAndInspectable(t *testing.T) {
	r := &fakeRunner{}
	m := Manager{Runner: r}
	if err := m.SetFirewallEnabled(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if err := m.SetFirewallEnabled(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	state, err := m.FirewallState(context.Background())
	if err != nil || !state.Enabled || !state.Active {
		t.Fatalf("state=%#v err=%v", state, err)
	}
	want := []call{
		{"systemctl", []string{"enable", FirewallService}},
		{"systemctl", []string{"disable", FirewallService}},
		{"systemctl", []string{"is-active", "--quiet", FirewallService}},
		{"systemctl", []string{"is-enabled", "--quiet", FirewallService}},
	}
	if !reflect.DeepEqual(r.calls, want) {
		t.Fatalf("calls=%#v want=%#v", r.calls, want)
	}
}
