package backup

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/config"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/models"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/notify"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/swcfg"
)

type Runner struct {
	log *slog.Logger
	st  *store.Store
	cfg config.Config
	mu  sync.Mutex

	progMu sync.Mutex
	lines  []string
	busy   bool

	pollPauser PollPauser
}

// PollPauser — приостановка SNMP-опроса на время бэкапа (снижение RAM / OOM).
type PollPauser interface {
	SetPollingPaused(bool)
}

func (r *Runner) Claim() bool {
	r.progMu.Lock()
	defer r.progMu.Unlock()
	if r.busy {
		return false
	}
	r.busy = true
	return true
}

func (r *Runner) Unclaim() {
	r.progMu.Lock()
	r.busy = false
	r.progMu.Unlock()
}

func (r *Runner) RecoverInterrupted(ctx context.Context) {
	CleanupStaleTemp(r.log)

	bs, err := r.st.GetBackupSettings(ctx)
	if err != nil {
		return
	}
	if bs.LastStatus == nil || strings.TrimSpace(*bs.LastStatus) != "running" {
		return
	}
	line := time.Now().Format("15:04:05") + "  прервано: служба NetLynx перезапущена"
	log := strings.TrimSpace(deref(bs.LastLog))
	if log != "" {
		log = log + "\n" + line
	} else {
		log = line
	}
	r.log.Info("backup: сброс зависшего running после рестарта")
	_ = r.st.SetBackupRunResult(ctx, "fail", "прервано: служба перезапущена", log)
}

func NewRunner(log *slog.Logger, st *store.Store, cfg config.Config) *Runner {
	if log == nil {
		log = slog.Default()
	}
	return &Runner{log: log, st: st, cfg: cfg}
}

func (r *Runner) SetPollPauser(p PollPauser) {
	r.pollPauser = p
}

func (r *Runner) setPollingPaused(v bool) {
	if r.pollPauser != nil {
		r.pollPauser.SetPollingPaused(v)
	}
}

func (r *Runner) Busy() bool {
	r.progMu.Lock()
	defer r.progMu.Unlock()
	return r.busy
}

func (r *Runner) Progress() (logText string, running bool) {
	r.progMu.Lock()
	defer r.progMu.Unlock()
	return strings.Join(r.lines, "\n"), r.busy
}

func (r *Runner) startProgress() {
	r.progMu.Lock()
	r.busy = true
	r.lines = nil
	r.progMu.Unlock()
}

func (r *Runner) stopProgress() {
	r.progMu.Lock()
	r.busy = false
	r.progMu.Unlock()
}

func (r *Runner) logText() string {
	r.progMu.Lock()
	defer r.progMu.Unlock()
	return strings.Join(r.lines, "\n")
}

func (r *Runner) note(ctx context.Context, msg string) {
	line := time.Now().Format("15:04:05") + "  " + msg
	r.progMu.Lock()
	r.lines = append(r.lines, line)
	if len(r.lines) > 400 {
		r.lines = r.lines[len(r.lines)-400:]
	}
	text := strings.Join(r.lines, "\n")
	r.progMu.Unlock()
	r.log.Info("backup", "step", msg)
	_ = r.st.SetBackupLog(ctx, text)
}

func (r *Runner) RunNow(ctx context.Context) error {
	if !r.mu.TryLock() {
		return fmt.Errorf("бэкап уже выполняется")
	}
	defer r.mu.Unlock()
	r.startProgress()
	defer r.stopProgress()
	r.note(ctx, "запуск резервного копирования")
	_ = r.st.SetBackupRunResult(ctx, "running", "", r.logText())
	status, errMsg := r.runLocked(ctx)
	if status == "ok" {
		r.note(ctx, "готово")
	} else if status == "partial" {
		r.note(ctx, "завершено частично: "+errMsg)
	} else {
		r.note(ctx, "ошибка: "+errMsg)
	}
	if setErr := r.st.SetBackupRunResult(ctx, status, errMsg, r.logText()); setErr != nil {
		r.log.Warn("backup status save", "err", setErr)
	}
	if status == "fail" {
		return fmt.Errorf("%s", errMsg)
	}
	return nil
}

func (r *Runner) runLocked(ctx context.Context) (status, errMsg string) {
	r.setPollingPaused(true)
	defer r.setPollingPaused(false)
	r.note(ctx, "опрос SNMP приостановлен на время бэкапа")

	bs, err := r.st.GetBackupSettings(ctx)
	if err != nil {
		return "fail", err.Error()
	}
	if !bs.LocalEnabled && !bs.EmailEnabled && !bs.ShareEnabled {
		return "fail", "не выбран ни один получатель (локально / почта / шара)"
	}

	now := time.Now()
	notes := []string{}

	var sshSwitches []models.Device
	if bs.SwitchCfgEnabled {
		devs, lerr := r.st.ListDevices(ctx)
		if lerr != nil {
			notes = append(notes, "список узлов: "+lerr.Error())
			r.note(ctx, "не удалось получить список узлов: "+lerr.Error())
		} else {
			var allSw []models.Device
			for _, d := range devs {
				if wantSwitchConfig(d) {
					allSw = append(allSw, d)
				}
			}
			onlineN := 0
			for _, d := range allSw {
				if d.IsOnline() {
					onlineN++
					sshSwitches = append(sshSwitches, d)
				}
			}
			r.note(ctx, fmt.Sprintf("Всего в базе %d узлов для SSH-бэкапа, онлайн - %d", len(allSw), onlineN))
			r.note(ctx, fmt.Sprintf("Бэкапим %d узлов (коммутаторы и RouterOS-роутеры)…", onlineN))
		}
	}

	sqlPath, sqlCleanup, err := DumpDatabaseFile(ctx, r.cfg.DatabaseURL, func(msg string) { r.note(ctx, msg) })
	if err != nil {
		return "fail", err.Error()
	}
	defer sqlCleanup()

	r.note(ctx, "читаю файл окружения сервера")
	envName, envData, envErr := ReadEnvFile()
	if envErr != nil {
		notes = append(notes, "файл окружения не найден: "+envErr.Error())
		r.note(ctx, "файл окружения не найден — в архив не войдёт")
		envName, envData = "", nil
	} else {
		r.note(ctx, "в архив добавлен "+envName)
	}

	cfgMap := map[string][]byte{}
	swErrs := map[string]string{}
	swOK := []string{}
	if bs.SwitchCfgEnabled {
		r.note(ctx, "снимаю конфиги по SSH (коммутаторы и RouterOS-роутеры)")
		if len(sshSwitches) == 0 {
			r.note(ctx, "нет онлайн-узлов для съёма конфига")
		} else {
			total := len(sshSwitches)
			for i, d := range sshSwitches {
				lab := labelDevice(d)
				pos := fmt.Sprintf("[%d/%d]", i+1, total)
				r.note(ctx, fmt.Sprintf("SSH %s %s (%s)…", pos, lab, d.Host))
				name, data, ferr := r.fetchOne(ctx, bs, d)
				if ferr != nil {
					swErrs[lab] = ferr.Error()
					r.note(ctx, fmt.Sprintf("SSH %s %s: ошибка — %s", pos, lab, ferr.Error()))
					continue
				}
				cfgMap[name] = data
				swOK = append(swOK, lab)
				r.note(ctx, fmt.Sprintf("SSH %s %s: ок (%d байт)", pos, lab, len(data)))
				if _, _, snapErr := r.st.SaveConfigSnapshotIfChanged(ctx, d.ID, string(data), "backup"); snapErr != nil {
					r.log.Warn("config snapshot from backup", "device_id", d.ID, "err", snapErr)
				}
				sys := ""
				if d.SysDescr != nil {
					sys = *d.SysDescr
				}
				if !swcfg.IsMikrotikRouterDevice(d.DeviceCategory, d.SSHVendor, sys, d.Name) {
					if res, syncErr := r.st.ApplyConfigPortRoles(ctx, d.ID, data); syncErr != nil {
						r.log.Warn("port roles from config", "device_id", d.ID, "err", syncErr)
					} else if res.Total() > 0 {
						r.note(ctx, fmt.Sprintf("SSH %s %s: из конфига — роли %d, описания %d", pos, lab, res.Roles, res.Descriptions))
					}
				}
			}
		}
	} else {
		r.note(ctx, "съём конфигов свитчей выключен")
	}

	manStatus := "ok"
	if len(swErrs) > 0 && len(swOK) > 0 {
		manStatus = "partial"
	} else if len(swErrs) > 0 && bs.SwitchCfgEnabled && len(swOK) == 0 {
		manStatus = "partial"
		notes = append(notes, "ни один конфиг свитча не снят")
	}

	r.note(ctx, "собираю ZIP")
	zipTmp, err := os.CreateTemp("", "netlynx-backup-*.zip")
	if err != nil {
		return "fail", "zip temp: " + err.Error()
	}
	zipPath := zipTmp.Name()
	_ = zipTmp.Close()
	defer os.Remove(zipPath)

	if err := BuildZipFile(zipPath, now, "", sqlPath, nil, envName, envData, cfgMap, Manifest{
		Status:       manStatus,
		Notes:        notes,
		SwitchErrors: swErrs,
		SwitchOK:     swOK,
	}); err != nil {
		return "fail", "zip: " + err.Error()
	}
	zipInfo, err := os.Stat(zipPath)
	if err != nil {
		return "fail", "zip stat: " + err.Error()
	}
	fname := ZipFileName(now)
	r.note(ctx, fmt.Sprintf("архив %s (%d КБ)", fname, zipInfo.Size()/1024))

	var destErrs []string
	if bs.LocalEnabled {
		r.note(ctx, "пишу на диск сервера: "+bs.LocalDir)
		if err := CopyLocalFile(bs.LocalDir, fname, zipPath); err != nil {
			destErrs = append(destErrs, "локально: "+err.Error())
			r.note(ctx, "локально: ошибка — "+err.Error())
		} else if err := RotateDir(bs.LocalDir, bs.LocalRetainDays, now); err != nil {
			destErrs = append(destErrs, "ротация локально: "+err.Error())
			r.note(ctx, "ротация локально: "+err.Error())
		} else {
			r.note(ctx, fmt.Sprintf("локально сохранено, хранить %d дн.", bs.LocalRetainDays))
		}
	}
	if bs.ShareEnabled {
		r.note(ctx, "копирую на шару")
		spec := ShareSpec{
			Kind:     bs.ShareKind,
			URL:      deref(bs.ShareURL),
			User:     deref(bs.ShareUsername),
			Password: deref(bs.SharePassword),
			Domain:   deref(bs.ShareDomain),
		}
		if err := DeliverShareFile(spec, fname, zipPath); err != nil {
			destErrs = append(destErrs, "шара: "+err.Error())
			r.note(ctx, "шара: ошибка — "+err.Error())
		} else if err := RotateShare(spec, bs.ShareRetainDays, now); err != nil {
			destErrs = append(destErrs, "ротация шары: "+err.Error())
			r.note(ctx, "ротация шары: "+err.Error())
		} else {
			r.note(ctx, fmt.Sprintf("шара: сохранено, хранить %d дн.", bs.ShareRetainDays))
		}
	}
	if bs.EmailEnabled {
		r.note(ctx, "отправляю письмо с вложением")
		zipBytes, readErr := os.ReadFile(zipPath)
		if readErr != nil {
			destErrs = append(destErrs, "почта: "+readErr.Error())
			r.note(ctx, "почта: ошибка — "+readErr.Error())
		} else if err := r.sendMail(ctx, bs, fname, zipBytes, manStatus, notes, swErrs); err != nil {
			destErrs = append(destErrs, "почта: "+err.Error())
			r.note(ctx, "почта: ошибка — "+err.Error())
		} else {
			r.note(ctx, "письмо отправлено")
		}
	}

	if len(destErrs) > 0 {
		msg := strings.Join(destErrs, "; ")
		if manStatus == "ok" {
			return "fail", msg
		}
		return "partial", msg
	}
	return manStatus, strings.Join(notes, "; ")
}

func (r *Runner) fetchOne(ctx context.Context, bs store.BackupSettings, d models.Device) (string, []byte, error) {
	_ = ctx
	user := strings.TrimSpace(deref(d.SSHUser))
	pass := deref(d.SSHPassword)
	enable := deref(d.SSHEnablePassword)
	port := bs.SSHPort
	if d.SSHPort != nil && *d.SSHPort > 0 {
		port = *d.SSHPort
	}
	if user == "" {
		user = strings.TrimSpace(deref(bs.SSHUser))
	}
	if strings.TrimSpace(pass) == "" {
		pass = deref(bs.SSHPassword)
	}
	if strings.TrimSpace(enable) == "" {
		enable = deref(bs.SSHEnablePassword)
	}
	if user == "" || pass == "" {
		return "", nil, fmt.Errorf("нет SSH-логина/пароля (карточка узла или общие настройки)")
	}
	kh := KnownHostsPath(r.cfg)
	timeout := time.Duration(bs.SSHTimeoutSeconds) * time.Second
	raw, err := swcfg.FetchConfig(swcfg.Creds{
		Host:       d.Host,
		Port:       port,
		User:       user,
		Password:   pass,
		EnablePass: enable,
		Vendor:     swcfg.Vendor(d.SSHVendor),
		SysDescr:   deref(d.SysDescr),
		Name:       d.Name,
		Timeout:    timeout,
		KnownHosts: kh,
	})
	if err != nil {
		return "", nil, err
	}
	return safeConfigName(d.Name, d.Host), raw, nil
}

func (r *Runner) sendMail(ctx context.Context, bs store.BackupSettings, fname string, zip []byte, status string, notes []string, swErrs map[string]string) error {
	ns, err := r.st.GetNotificationSettings(ctx)
	if err != nil {
		return err
	}
	to := strings.TrimSpace(deref(bs.EmailTo))
	if to == "" {
		to = deref(ns.EmailTo)
	}
	cfg := notify.EmailConfig{
		From:          deref(ns.EmailFrom),
		To:            to,
		SMTPHost:      deref(ns.SMTPHost),
		SMTPPort:      ns.SMTPPort,
		SMTPUsername:  deref(ns.SMTPUsername),
		SMTPPassword:  deref(ns.SMTPPassword),
		TLSSkipVerify: ns.SMTPTLSSkipVerify,
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Резервная копия NetLynx: %s\n", status)
	for _, n := range notes {
		fmt.Fprintf(&b, "- %s\n", n)
	}
	for k, v := range swErrs {
		fmt.Fprintf(&b, "свитч %s: %s\n", k, v)
	}
	return notify.NewEmail().SendWithAttachments(ctx, cfg, "NetLynx backup "+fname, b.String(), []notify.Attachment{{
		Name: fname,
		Data: zip,
	}})
}

// WantConfigBackup — SSH-съём running-config (коммутаторы + MikroTik router).
func WantConfigBackup(d models.Device) bool {
	return wantConfigBackup(d)
}

func wantSwitchConfig(d models.Device) bool {
	return wantConfigBackup(d)
}

// wantConfigBackup — SSH-съём конфига в архив: коммутаторы и роутеры с явным вендором MikroTik.
func wantConfigBackup(d models.Device) bool {
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

func labelDevice(d models.Device) string {
	if strings.TrimSpace(d.Name) != "" {
		return d.Name
	}
	return d.Host
}

func safeConfigName(name, host string) string {
	base := strings.TrimSpace(name)
	if base == "" {
		base = host
	}
	var b strings.Builder
	for _, r := range base {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "" {
		out = "device"
	}
	host = strings.TrimSpace(host)
	if host != "" && !strings.Contains(out, host) {
		out += "-" + host
	}
	return out + ".cfg"
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func KnownHostsPath(cfg config.Config) string {
	if strings.TrimSpace(cfg.SSHPOEKnownHosts) != "" {
		return cfg.SSHPOEKnownHosts
	}
	return filepath.ToSlash("/var/lib/netlynx/ssh_known_hosts")
}
