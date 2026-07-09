package systemd

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/calmcacil/cnftctl/internal/nft"
)

type Manager struct {
	Runner nft.Runner
}

func (m Manager) runner() nft.Runner {
	if m.Runner != nil {
		return m.Runner
	}
	return nft.ExecRunner{}
}

func (m Manager) StartRollback(ctx context.Context, unitName, scriptPath string, delay time.Duration) error {
	if unitName == "" || scriptPath == "" {
		return errors.New("unit name and rollback script path are required")
	}
	if delay <= 0 {
		return errors.New("rollback delay must be positive")
	}
	seconds := strconv.FormatInt(int64(delay/time.Second), 10)
	res := m.runner().Run(ctx, "systemd-run", "--unit", unitName, "--on-active", seconds, "--collect", scriptPath)
	if !res.OK() {
		return fmt.Errorf("start rollback unit %s: %w", unitName, res.Error())
	}
	return nil
}

func (m Manager) Cancel(ctx context.Context, unitName string) error {
	if unitName == "" {
		return errors.New("unit name is required")
	}
	res := m.runner().Run(ctx, "systemctl", "cancel", unitName)
	if !res.OK() {
		return fmt.Errorf("cancel rollback unit %s: %w", unitName, res.Error())
	}
	return nil
}

func (m Manager) IsActive(ctx context.Context, unitName string) (bool, error) {
	if unitName == "" {
		return false, errors.New("unit name is required")
	}
	res := m.runner().Run(ctx, "systemctl", "is-active", "--quiet", unitName)
	return res.OK(), nil
}
