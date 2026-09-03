package configsnapshot

import (
	"context"
	"fmt"
	"strings"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/config"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/devssh"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/models"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/swcfg"
)

func ShouldSnapshotDevice(d models.Device) bool {
	if strings.TrimSpace(d.Host) == "" {
		return false
	}
	cat := store.NormalizeDeviceCategory(d.DeviceCategory)
	if cat == store.DeviceCategorySwitch {
		return true
	}
	return cat == store.DeviceCategoryRouter &&
		swcfg.IsMikrotikRouterForConfigBackup(d.DeviceCategory, d.SSHVendor)
}

func SaveIfChanged(ctx context.Context, st *store.Store, deviceID int64, text, source string) (saved bool, id int64, err error) {
	return st.SaveConfigSnapshotIfChanged(ctx, deviceID, text, source)
}

func FetchAndStore(ctx context.Context, st *store.Store, cfg config.Config, bs store.BackupSettings, dev *models.Device, source string) (saved bool, id int64, err error) {
	if dev == nil {
		return false, 0, fmt.Errorf("nil device")
	}
	if !ShouldSnapshotDevice(*dev) {
		return false, 0, fmt.Errorf("узел не поддерживает съём конфига")
	}
	user, pass, enable, port, timeout := devssh.ResolveDevice(dev, bs, cfg)
	if strings.TrimSpace(user) == "" || strings.TrimSpace(pass) == "" {
		return false, 0, fmt.Errorf("нет SSH-логина/пароля")
	}
	sys := ""
	if dev.SysDescr != nil {
		sys = *dev.SysDescr
	}
	raw, err := swcfg.FetchConfig(swcfg.Creds{
		Host:       dev.Host,
		Port:       port,
		User:       user,
		Password:   pass,
		EnablePass: enable,
		Vendor:     swcfg.Vendor(dev.SSHVendor),
		SysDescr:   sys,
		Name:       dev.Name,
		Timeout:    timeout,
		KnownHosts: devssh.KnownHostsPath(cfg),
	})
	if err != nil {
		return false, 0, err
	}
	return SaveIfChanged(ctx, st, dev.ID, string(raw), source)
}

func PruneOld(ctx context.Context, st *store.Store, retainDays int) (int64, error) {
	if retainDays <= 0 {
		retainDays = 90
	}
	return st.PruneConfigSnapshots(ctx, time.Now().Add(-time.Duration(retainDays)*24*time.Hour))
}
