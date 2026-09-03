package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/backup"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
	"github.com/go-chi/chi/v5"
)

type publicBackupSettings struct {
	ScheduleEnabled   bool       `json:"schedule_enabled"`
	ScheduleHour      int        `json:"schedule_hour"`
	ScheduleMinute    int        `json:"schedule_minute"`
	LocalEnabled      bool       `json:"local_enabled"`
	LocalDir          string     `json:"local_dir"`
	LocalRetainDays   int        `json:"local_retain_days"`
	EmailEnabled      bool       `json:"email_enabled"`
	EmailTo           *string    `json:"email_to"`
	ShareEnabled      bool       `json:"share_enabled"`
	ShareKind         string     `json:"share_kind"`
	ShareURL          *string    `json:"share_url"`
	ShareUsername     *string    `json:"share_username"`
	ShareDomain       *string    `json:"share_domain"`
	ShareRetainDays   int        `json:"share_retain_days"`
	HasSharePassword  bool       `json:"has_share_password"`
	SwitchCfgEnabled  bool       `json:"switch_cfg_enabled"`
	SSHUser           *string    `json:"ssh_user"`
	SSHPort           int        `json:"ssh_port"`
	SSHTimeoutSeconds int        `json:"ssh_timeout_seconds"`
	HasSSHPassword    bool       `json:"has_ssh_password"`
	HasSSHEnablePass  bool       `json:"has_ssh_enable_password"`
	LastRunAt         *time.Time `json:"last_run_at,omitempty"`
	LastStatus        *string    `json:"last_status,omitempty"`
	LastError         *string    `json:"last_error,omitempty"`
	LastLog           *string    `json:"last_log,omitempty"`
	LiveDeviceCount   int        `json:"live_device_count"`
	JobRunning        bool       `json:"job_running"`
	ProcessStartedAt  time.Time  `json:"process_started_at"`
	AppVersion        string     `json:"app_version,omitempty"`
}

func toPublicBackup(b store.BackupSettings) publicBackupSettings {
	return publicBackupSettings{
		ScheduleEnabled:   b.ScheduleEnabled,
		ScheduleHour:      b.ScheduleHour,
		ScheduleMinute:    b.ScheduleMinute,
		LocalEnabled:      b.LocalEnabled,
		LocalDir:          b.LocalDir,
		LocalRetainDays:   b.LocalRetainDays,
		EmailEnabled:      b.EmailEnabled,
		EmailTo:           b.EmailTo,
		ShareEnabled:      b.ShareEnabled,
		ShareKind:         b.ShareKind,
		ShareURL:          b.ShareURL,
		ShareUsername:     b.ShareUsername,
		ShareDomain:       b.ShareDomain,
		ShareRetainDays:   b.ShareRetainDays,
		HasSharePassword:  b.SharePassword != nil && strings.TrimSpace(*b.SharePassword) != "",
		SwitchCfgEnabled:  b.SwitchCfgEnabled,
		SSHUser:           b.SSHUser,
		SSHPort:           b.SSHPort,
		SSHTimeoutSeconds: b.SSHTimeoutSeconds,
		HasSSHPassword:    b.SSHPassword != nil && strings.TrimSpace(*b.SSHPassword) != "",
		HasSSHEnablePass:  b.SSHEnablePassword != nil && strings.TrimSpace(*b.SSHEnablePassword) != "",
		LastRunAt:         b.LastRunAt,
		LastStatus:        b.LastStatus,
		LastError:         b.LastError,
		LastLog:           b.LastLog,
		LiveDeviceCount:   0,
	}
}

func (s *Server) handleGetBackupSettings(w http.ResponseWriter, r *http.Request) {
	row, err := s.st.GetBackupSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	pub := toPublicBackup(row)
	if n, err := s.st.CountDevices(r.Context()); err == nil {
		pub.LiveDeviceCount = n
	}
	pub.ProcessStartedAt = startedAt.UTC()
	pub.AppVersion = s.bi.Version
	if s.backupRun != nil {
		text, running := s.backupRun.Progress()
		pub.JobRunning = running
		if running {
			st := "running"
			pub.LastStatus = &st
			if strings.TrimSpace(text) != "" {
				pub.LastLog = &text
			}
		} else if row.LastStatus != nil && strings.TrimSpace(*row.LastStatus) == "running" {
			s.backupRun.RecoverInterrupted(r.Context())
			if row2, err := s.st.GetBackupSettings(r.Context()); err == nil {
				n := pub.LiveDeviceCount
				pub = toPublicBackup(row2)
				pub.LiveDeviceCount = n
			}
			pub.JobRunning = false
			pub.ProcessStartedAt = startedAt.UTC()
			pub.AppVersion = s.bi.Version
		}
	}
	writeJSON(w, http.StatusOK, pub)
}

type patchBackupBody struct {
	ScheduleEnabled   *bool   `json:"schedule_enabled"`
	ScheduleHour      *int    `json:"schedule_hour"`
	ScheduleMinute    *int    `json:"schedule_minute"`
	LocalEnabled      *bool   `json:"local_enabled"`
	LocalDir          *string `json:"local_dir"`
	LocalRetainDays   *int    `json:"local_retain_days"`
	EmailEnabled      *bool   `json:"email_enabled"`
	EmailTo           *string `json:"email_to"`
	ShareEnabled      *bool   `json:"share_enabled"`
	ShareKind         *string `json:"share_kind"`
	ShareURL          *string `json:"share_url"`
	ShareUsername     *string `json:"share_username"`
	SharePassword     *string `json:"share_password"`
	ShareDomain       *string `json:"share_domain"`
	ShareRetainDays   *int    `json:"share_retain_days"`
	SwitchCfgEnabled  *bool   `json:"switch_cfg_enabled"`
	SSHUser           *string `json:"ssh_user"`
	SSHPassword       *string `json:"ssh_password"`
	SSHPort           *int    `json:"ssh_port"`
	SSHEnablePassword *string `json:"ssh_enable_password"`
	SSHTimeoutSeconds *int    `json:"ssh_timeout_seconds"`
}

func (s *Server) handlePatchBackupSettings(w http.ResponseWriter, r *http.Request) {
	cur, err := s.st.GetBackupSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var body patchBackupBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON: "+err.Error())
		return
	}
	if body.ScheduleEnabled != nil {
		cur.ScheduleEnabled = *body.ScheduleEnabled
	}
	if body.ScheduleHour != nil {
		cur.ScheduleHour = *body.ScheduleHour
	}
	if body.ScheduleMinute != nil {
		cur.ScheduleMinute = *body.ScheduleMinute
	}
	if body.LocalEnabled != nil {
		cur.LocalEnabled = *body.LocalEnabled
	}
	if body.LocalDir != nil {
		d := strings.TrimSpace(*body.LocalDir)
		if d == "" {
			d = "/var/backups/netlynx"
		}
		cur.LocalDir = d
	}
	if body.LocalRetainDays != nil {
		cur.LocalRetainDays = *body.LocalRetainDays
	}
	if body.EmailEnabled != nil {
		cur.EmailEnabled = *body.EmailEnabled
	}
	if body.EmailTo != nil {
		t := strings.TrimSpace(*body.EmailTo)
		if t == "" {
			cur.EmailTo = nil
		} else {
			cur.EmailTo = &t
		}
	}
	if body.ShareEnabled != nil {
		cur.ShareEnabled = *body.ShareEnabled
	}
	if body.ShareKind != nil {
		k := strings.ToLower(strings.TrimSpace(*body.ShareKind))
		if k != "nfs" {
			k = "smb"
		}
		cur.ShareKind = k
	}
	if body.ShareURL != nil {
		t := strings.TrimSpace(*body.ShareURL)
		if t == "" {
			cur.ShareURL = nil
		} else {
			cur.ShareURL = &t
		}
	}
	if body.ShareUsername != nil {
		t := strings.TrimSpace(*body.ShareUsername)
		if t == "" {
			cur.ShareUsername = nil
		} else {
			cur.ShareUsername = &t
		}
	}
	if body.SharePassword != nil && strings.TrimSpace(*body.SharePassword) != "" {
		p := *body.SharePassword
		cur.SharePassword = &p
	}
	if body.ShareDomain != nil {
		t := strings.TrimSpace(*body.ShareDomain)
		if t == "" {
			cur.ShareDomain = nil
		} else {
			cur.ShareDomain = &t
		}
	}
	if body.ShareRetainDays != nil {
		cur.ShareRetainDays = *body.ShareRetainDays
	}
	if body.SwitchCfgEnabled != nil {
		cur.SwitchCfgEnabled = *body.SwitchCfgEnabled
	}
	if body.SSHUser != nil {
		t := strings.TrimSpace(*body.SSHUser)
		if t == "" {
			cur.SSHUser = nil
		} else {
			cur.SSHUser = &t
		}
	}
	if body.SSHPassword != nil && strings.TrimSpace(*body.SSHPassword) != "" {
		p := *body.SSHPassword
		cur.SSHPassword = &p
	}
	if body.SSHPort != nil {
		cur.SSHPort = *body.SSHPort
	}
	if body.SSHEnablePassword != nil && strings.TrimSpace(*body.SSHEnablePassword) != "" {
		p := *body.SSHEnablePassword
		cur.SSHEnablePassword = &p
	}
	if body.SSHTimeoutSeconds != nil {
		cur.SSHTimeoutSeconds = *body.SSHTimeoutSeconds
	}
	if err := s.st.UpsertBackupSettings(r.Context(), cur); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.audit(r, "settings.backup.update", "settings", nil, nil)
	s.handleGetBackupSettings(w, r)
}

func (s *Server) handleRunBackup(w http.ResponseWriter, r *http.Request) {
	if s.backupRun == nil {
		writeError(w, http.StatusInternalServerError, "backup runner не инициализирован")
		return
	}
	if !s.backupRun.Claim() {
		writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "started": false, "already": true})
		return
	}
	s.audit(r, "backup.run", "settings", nil, nil)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Minute)
		defer cancel()
		_ = s.backupRun.RunNow(ctx)
	}()
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "started": true})
}

func (s *Server) handleListBackupArchives(w http.ResponseWriter, r *http.Request) {
	if s.backupRun == nil {
		writeError(w, http.StatusInternalServerError, "backup runner не инициализирован")
		return
	}
	list, err := s.backupRun.ListArchives(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []backup.ArchiveInfo{}
	}
	writeJSON(w, http.StatusOK, list)
}

type backupFileBody struct {
	Filename string `json:"filename"`
	Confirm  string `json:"confirm"`
}

func (s *Server) handleVerifyBackup(w http.ResponseWriter, r *http.Request) {
	s.startBackupZipJob(w, r, false)
}

func (s *Server) handleImportBackup(w http.ResponseWriter, r *http.Request) {
	s.startBackupZipJob(w, r, true)
}

func (s *Server) startBackupZipJob(w http.ResponseWriter, r *http.Request, doImport bool) {
	if s.backupRun == nil {
		writeError(w, http.StatusInternalServerError, "backup runner не инициализирован")
		return
	}
	if !s.backupRun.Claim() {
		writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "started": false, "already": true})
		return
	}
	path, cleanup, confirm, err := s.backupZipFromRequest(r)
	if err != nil {
		s.backupRun.Unclaim()
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if doImport && strings.ToUpper(strings.TrimSpace(confirm)) != "IMPORT" {
		s.backupRun.Unclaim()
		cleanup()
		writeError(w, http.StatusBadRequest, "для импорта введите подтверждение IMPORT")
		return
	}
	if doImport {
		s.audit(r, "backup.import", "settings", nil, map[string]interface{}{"file": path})
	} else {
		s.audit(r, "backup.verify", "settings", nil, map[string]interface{}{"file": path})
	}
	go func() {
		defer cleanup()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		if doImport {
			_ = s.backupRun.ImportArchive(ctx, path)
			return
		}
		_ = s.backupRun.VerifyArchive(ctx, path)
	}()
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "started": true})
}

func (s *Server) backupZipFromRequest(r *http.Request) (path string, cleanup func(), confirm string, err error) {
	cleanup = func() {}
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/form-data") {
		if err := r.ParseMultipartForm(512 << 20); err != nil {
			return "", cleanup, "", fmtBackupErr("upload: ", err)
		}
		confirm = r.FormValue("confirm")
		f, hdr, ferr := r.FormFile("file")
		if ferr == nil {
			defer f.Close()
			tmp, e := os.CreateTemp("", "invetor-upload-*.zip")
			if e != nil {
				return "", cleanup, confirm, e
			}
			if _, e := io.Copy(tmp, io.LimitReader(f, 512<<20)); e != nil {
				_ = tmp.Close()
				_ = os.Remove(tmp.Name())
				return "", cleanup, confirm, e
			}
			_ = tmp.Close()
			_ = hdr
			return tmp.Name(), func() { _ = os.Remove(tmp.Name()) }, confirm, nil
		}
		name := strings.TrimSpace(r.FormValue("filename"))
		return s.localBackupZip(r.Context(), name, confirm)
	}
	var body backupFileBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return "", cleanup, "", fmt.Errorf("ожидается JSON {filename} или ZIP в поле file")
	}
	return s.localBackupZip(r.Context(), body.Filename, body.Confirm)
}

func (s *Server) localBackupZip(ctx context.Context, name, confirm string) (string, func(), string, error) {
	noop := func() {}
	bs, err := s.st.GetBackupSettings(ctx)
	if err != nil {
		return "", noop, confirm, err
	}
	p, err := backup.SafeArchivePath(bs.LocalDir, name)
	if err != nil {
		return "", noop, confirm, err
	}
	return p, noop, confirm, nil
}

func fmtBackupErr(prefix string, err error) error {
	return errors.New(prefix + err.Error())
}

type patchDeviceSSHBody struct {
	SSHUser           *string `json:"ssh_user"`
	SSHPassword       *string `json:"ssh_password"`
	SSHPort           *int    `json:"ssh_port"`
	SSHEnablePassword *string `json:"ssh_enable_password"`
	SSHVendor         *string `json:"ssh_vendor"`
}

func (s *Server) handlePatchDeviceSSH(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "неверный id")
		return
	}
	var body patchDeviceSSHBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON: "+err.Error())
		return
	}
	in := store.DeviceSSHInput{
		SSHUser:           body.SSHUser,
		SSHPassword:       body.SSHPassword,
		SSHPort:           body.SSHPort,
		SSHEnablePassword: body.SSHEnablePassword,
	}
	if body.SSHVendor != nil {
		in.SSHVendor = store.NormalizeSSHVendor(*body.SSHVendor)
	}
	if err := s.st.UpdateDeviceSSH(r.Context(), id, in); err != nil {
		if errors.Is(err, store.ErrDeviceNotFound) {
			writeError(w, http.StatusNotFound, "узел не найден")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.audit(r, "device.ssh.update", "device", &id, nil)
	d, err := s.st.GetDevice(r.Context(), id)
	if err != nil || d == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
		return
	}
	redactDeviceForAPI(d)
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) RunBackupScheduler(ctx context.Context) {
	if s.backupRun == nil {
		return
	}
	s.backupRun.Scheduler(ctx)
}
