package systemd

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/calmcacil/cnftctl/internal/nft"
)

type call struct {
	name string
	args []string
}
type fakeRunner struct{ calls []call }

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) nft.Result {
	f.calls = append(f.calls, call{name: name, args: append([]string(nil), args...)})
	return nft.Result{}
}

func TestStartRollbackUsesTransientUnit(t *testing.T) {
	r := &fakeRunner{}
	m := Manager{Runner: r}
	if err := m.StartRollback(context.Background(), "cnftctl-rollback-abc", "/run/cnftctl/abc/rollback.sh", 120*time.Second); err != nil {
		t.Fatal(err)
	}
	want := []call{{name: "systemd-run", args: []string{"--unit", "cnftctl-rollback-abc", "--on-active", "120", "--collect", "/run/cnftctl/abc/rollback.sh"}}}
	if !reflect.DeepEqual(r.calls, want) {
		t.Fatalf("calls = %#v, want %#v", r.calls, want)
	}
}

func TestCancelUsesSystemctl(t *testing.T) {
	r := &fakeRunner{}
	if err := (Manager{Runner: r}).Cancel(context.Background(), "cnftctl-rollback-abc"); err != nil {
		t.Fatal(err)
	}
	want := []call{{name: "systemctl", args: []string{"cancel", "cnftctl-rollback-abc"}}}
	if !reflect.DeepEqual(r.calls, want) {
		t.Fatalf("calls = %#v, want %#v", r.calls, want)
	}
}
