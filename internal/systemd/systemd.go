package systemd

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/calmcacil/cnftctl/internal/nft"
)

const (
	FirewallService  = "cnftctl-firewall.service"
	RollbackPrefix   = "cnftctl-rollback@"
	DDNSRefreshTimer = "cnftctl-ddns-refresh.timer"
)

var safeInstance = regexp.MustCompile(`^[a-f0-9]{32}$`)

type Manager struct{ Runner nft.Runner }

type UnitState struct{ Enabled, Active bool }

func (m Manager) ReconcileDDNSTimer(ctx context.Context, desired bool) error {
	args := []string{"disable", "--now", DDNSRefreshTimer}
	if desired {
		args = []string{"enable", "--now", DDNSRefreshTimer}
	}
	if res := m.runner().Run(ctx, "systemctl", args...); !res.OK() {
		return fmt.Errorf("reconcile %s: %w", DDNSRefreshTimer, res.Error())
	}
	return nil
}

func (m Manager) DDNSState(ctx context.Context) (UnitState, error) {
	active, err := m.IsActive(ctx, DDNSRefreshTimer)
	if err != nil {
		return UnitState{}, err
	}
	res := m.runner().Run(ctx, "systemctl", "is-enabled", "--quiet", DDNSRefreshTimer)
	if res.OK() {
		return UnitState{Enabled: true, Active: active}, nil
	}
	if res.Stderr == "" {
		return UnitState{Active: active}, nil
	}
	return UnitState{}, fmt.Errorf("inspect enablement of %s: %w", DDNSRefreshTimer, res.Error())
}

func (m Manager) runner() nft.Runner {
	if m.Runner != nil {
		return m.Runner
	}
	return nft.ExecRunner{}
}

func RollbackTimer(id string) (string, error) {
	if !safeInstance.MatchString(id) {
		return "", errors.New("invalid rollback transaction ID")
	}
	return RollbackPrefix + id + ".timer", nil
}

// ArmRollback starts the pre-installed timer and verifies that systemd considers it active.
func (m Manager) ArmRollback(ctx context.Context, id string) error {
	unit, err := RollbackTimer(id)
	if err != nil {
		return err
	}
	if res := m.runner().Run(ctx, "systemctl", "start", unit); !res.OK() {
		return fmt.Errorf("arm rollback timer %s: %w", unit, res.Error())
	}
	active, err := m.IsActive(ctx, unit)
	if err != nil {
		return err
	}
	if !active {
		return fmt.Errorf("rollback timer %s is not active", unit)
	}
	return nil
}

func (m Manager) ActivateFirewall(ctx context.Context) error {
	res := m.runner().Run(ctx, "systemctl", "restart", FirewallService)
	if !res.OK() {
		return fmt.Errorf("activate %s: %w", FirewallService, res.Error())
	}
	return nil
}

func (m Manager) FirewallState(ctx context.Context) (UnitState, error) {
	active, err := m.IsActive(ctx, FirewallService)
	if err != nil {
		return UnitState{}, err
	}
	res := m.runner().Run(ctx, "systemctl", "is-enabled", "--quiet", FirewallService)
	if res.OK() {
		return UnitState{Enabled: true, Active: active}, nil
	}
	if res.Stderr == "" {
		return UnitState{Active: active}, nil
	}
	return UnitState{}, fmt.Errorf("inspect enablement of %s: %w", FirewallService, res.Error())
}

func (m Manager) SetFirewallEnabled(ctx context.Context, enabled bool) error {
	action := "disable"
	if enabled {
		action = "enable"
	}
	if res := m.runner().Run(ctx, "systemctl", action, FirewallService); !res.OK() {
		return fmt.Errorf("%s %s: %w", action, FirewallService, res.Error())
	}
	return nil
}

func (m Manager) CancelRollback(ctx context.Context, id string) error {
	unit, err := RollbackTimer(id)
	if err != nil {
		return err
	}
	res := m.runner().Run(ctx, "systemctl", "stop", unit)
	if !res.OK() {
		return fmt.Errorf("stop rollback timer %s: %w", unit, res.Error())
	}
	return nil
}

func (m Manager) IsActive(ctx context.Context, unit string) (bool, error) {
	if unit == "" {
		return false, errors.New("unit name is required")
	}
	res := m.runner().Run(ctx, "systemctl", "is-active", "--quiet", unit)
	if res.OK() {
		return true, nil
	}
	// is-active uses exit status 3 for an inactive unit; stderr indicates operational errors.
	if res.Stderr == "" {
		return false, nil
	}
	return false, fmt.Errorf("inspect systemd unit %s: %w", unit, res.Error())
}
