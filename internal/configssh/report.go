package configssh

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/config"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/devssh"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/live"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/models"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/notify"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/swcfg"
)

const (
	EventConfigSSHFail     = "CONFIG_SSH_FAIL"
	EventBackupSSHPartial  = "BACKUP_SSH_PARTIAL"
	failDebounce           = 6 * time.Hour
)

// Reporter пишет события сбоев SSH-съёма конфига и шлёт в Telegram/email через EventHook.
type Reporter struct {
	log  *slog.Logger
	st   *store.Store
	cfg  config.Config
	hub  *live.Hub
	hook *notify.EventHook
}

func NewReporter(log *slog.Logger, st *store.Store, cfg config.Config, hub *live.Hub, hook *notify.EventHook) *Reporter {
	if log == nil {
		log = slog.Default()
	}
	return &Reporter{log: log, st: st, cfg: cfg, hub: hub, hook: hook}
}

// ReportFail — CONFIG_SSH_FAIL с debounce 6ч на (device, err_class).
func (r *Reporter) ReportFail(ctx context.Context, deviceID int64, name, host string, err error, source string) {
	if r == nil || r.st == nil || deviceID <= 0 || err == nil {
		return
	}
	errClass := swcfg.ClassifySSHError(err)
	if errClass == "" {
		errClass = "other"
	}
	source = strings.TrimSpace(source)
	if source == "" {
		source = "other"
	}
	since := time.Now().UTC().Add(-failDebounce)
	dup, qerr := r.st.HasConfigSSHFailSince(ctx, deviceID, EventConfigSSHFail, errClass, since)
	if qerr != nil {
		r.log.Warn("config ssh fail debounce", "err", qerr)
	} else if dup {
		return
	}
	pl := map[string]interface{}{
		"host":      strings.TrimSpace(host),
		"name":      strings.TrimSpace(name),
		"err":       err.Error(),
		"err_class": errClass,
		"source":    source,
	}
	evID, ierr := r.st.InsertEvent(ctx, deviceID, nil, EventConfigSSHFail, "warning", pl)
	if ierr != nil {
		r.log.Warn("config ssh fail insert", "err", ierr)
		return
	}
	if r.hub != nil {
		r.hub.Publish(live.EventPayload{
			EventID: evID, DeviceID: deviceID, DeviceName: name, DeviceHost: host,
			EventType: EventConfigSSHFail, Severity: "warning", Payload: pl,
		})
	}
	if r.hook != nil {
		r.hook.DispatchEvent(deviceID, name, host, evID, nil, EventConfigSSHFail, "warning", pl)
	}
}

// ReportBackupPartial — сводка после ZIP-рана со сбоями SSH.
func (r *Reporter) ReportBackupPartial(ctx context.Context, failedCount int, samples []string) {
	if r == nil || r.st == nil || failedCount <= 0 {
		return
	}
	since := time.Now().UTC().Add(-failDebounce)
	dup, err := r.st.HasEventTypeSince(ctx, EventBackupSSHPartial, since)
	if err != nil {
		r.log.Warn("backup ssh partial debounce", "err", err)
	} else if dup {
		return
	}
	pl := map[string]interface{}{
		"failed_count": failedCount,
		"samples":      samples,
		"source":       "backup",
	}
	// device_id=0 — системное событие (как SERVICE_STARTED).
	evID, ierr := r.st.InsertEvent(ctx, 0, nil, EventBackupSSHPartial, "warning", pl)
	if ierr != nil {
		r.log.Warn("backup ssh partial insert", "err", ierr)
		return
	}
	if r.hub != nil {
		r.hub.Publish(live.EventPayload{
			EventID: evID, EventType: EventBackupSSHPartial, Severity: "warning", Payload: pl,
		})
	}
	if r.hook != nil {
		r.hook.DispatchEvent(0, "NetLynx", "", evID, nil, EventBackupSSHPartial, "warning", pl)
	}
}

// OnboardDevice — EnsureHostKey + пробный Dial; при fail — ReportFail. Не блокирует создание узла.
func (r *Reporter) OnboardDevice(ctx context.Context, dev *models.Device) (warning string) {
	if r == nil || r.st == nil || dev == nil {
		return ""
	}
	host := strings.TrimSpace(dev.Host)
	if host == "" {
		return ""
	}
	bs, err := r.st.GetBackupSettings(ctx)
	if err != nil {
		return ""
	}
	kh := devssh.KnownHostsPath(r.cfg)
	user, pass, enable, port, timeout := devssh.ResolveDevice(dev, bs, r.cfg)
	if err := swcfg.EnsureHostKey(kh, host, port, timeout); err != nil {
		r.ReportFail(ctx, dev.ID, dev.Name, host, err, "onboard")
		return "SSH host key: " + err.Error()
	}
	if strings.TrimSpace(user) == "" || strings.TrimSpace(pass) == "" {
		return ""
	}
	sys := ""
	if dev.SysDescr != nil {
		sys = *dev.SysDescr
	}
	client, derr := swcfg.DialSwitch(swcfg.Creds{
		Host:       host,
		Port:       port,
		User:       user,
		Password:   pass,
		EnablePass: enable,
		Vendor:     swcfg.Vendor(dev.SSHVendor),
		SysDescr:   sys,
		Name:       dev.Name,
		Timeout:    timeout,
		KnownHosts: kh,
	})
	if derr != nil {
		r.ReportFail(ctx, dev.ID, dev.Name, host, derr, "onboard")
		return "SSH: " + derr.Error()
	}
	_ = client.Close()
	return ""
}
