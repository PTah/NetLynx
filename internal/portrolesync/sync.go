package portrolesync

import (
	"context"
	"fmt"
	"strings"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/backup"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/config"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/models"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/swcfg"
)

// SyncDevicePortRoles читает show run (если configRaw nil) и обновляет port_role + description из CLI.
type SyncOpts struct {
	// Force — всегда читать show run по SSH, игнорируя свежий cli_mode_synced_at.
	Force bool
	// MaxAge — при Force=false не ходить на SSH, если роли синхронизированы недавнее (0 = без лимита).
	MaxAge time.Duration
}

type SyncResult struct {
	store.ConfigPortApplyResult
	Skipped   bool
	SyncedAt  *time.Time
	ConfigRaw []byte // running-config, прочитанный или переданный в sync (для кэша настроек порта).
}

func SyncDevicePortRoles(
	ctx context.Context,
	st *store.Store,
	cfg config.Config,
	bs store.BackupSettings,
	dev *models.Device,
	configRaw []byte,
	opts SyncOpts,
) (SyncResult, error) {
	var empty SyncResult
	if st == nil || dev == nil {
		return empty, fmt.Errorf("sync: nil store or device")
	}
	if len(configRaw) == 0 && !opts.Force && opts.MaxAge > 0 {
		syncedAt, err := st.GetDeviceCLIModeSyncAt(ctx, dev.ID)
		if err != nil {
			return empty, err
		}
		if syncedAt != nil && time.Since(*syncedAt) < opts.MaxAge {
			return SyncResult{Skipped: true, SyncedAt: syncedAt}, nil
		}
	}
	raw := configRaw
	if len(raw) == 0 {
		user, pass, enable, port, timeout := ResolveDeviceSSH(dev, bs, cfg)
		if strings.TrimSpace(user) == "" || strings.TrimSpace(pass) == "" {
			return empty, fmt.Errorf("нет SSH-логина/пароля (карточка узла или настройки бэкапа)")
		}
		sys := ""
		if dev.SysDescr != nil {
			sys = *dev.SysDescr
		}
		var err error
		raw, err = swcfg.FetchConfig(swcfg.Creds{
			Host:       dev.Host,
			Port:       port,
			User:       user,
			Password:   pass,
			EnablePass: enable,
			Vendor:     swcfg.Vendor(dev.SSHVendor),
			SysDescr:   sys,
			Name:       dev.Name,
			Timeout:    timeout,
			KnownHosts: backup.KnownHostsPath(cfg),
		})
		if err != nil {
			return empty, fmt.Errorf("show running-config: %w", err)
		}
	}
	if len(raw) > 0 {
		if _, _, snapErr := st.SaveConfigSnapshotIfChanged(ctx, dev.ID, string(raw), "port_sync"); snapErr != nil {
			// не блокируем sync ролей
			_ = snapErr
		}
	}
	applied, err := st.ApplyConfigPortRoles(ctx, dev.ID, raw)
	if err != nil {
		return empty, err
	}
	syncedAt, _ := st.GetDeviceCLIModeSyncAt(ctx, dev.ID)
	return SyncResult{ConfigPortApplyResult: applied, SyncedAt: syncedAt, ConfigRaw: raw}, nil
}
