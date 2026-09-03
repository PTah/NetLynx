package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type BackupSettings struct {
	ScheduleEnabled    bool
	ScheduleHour       int
	ScheduleMinute     int
	LocalEnabled       bool
	LocalDir           string
	LocalRetainDays    int
	EmailEnabled       bool
	EmailTo            *string
	ShareEnabled       bool
	ShareKind          string
	ShareURL           *string
	ShareUsername      *string
	SharePassword      *string
	ShareDomain        *string
	ShareRetainDays    int
	SwitchCfgEnabled   bool
	SSHUser            *string
	SSHPassword        *string
	SSHPort            int
	SSHEnablePassword  *string
	SSHTimeoutSeconds  int
	LastRunAt          *time.Time
	LastStatus         *string
	LastError          *string
	LastLog            *string
}

func defaultBackupSettings() BackupSettings {
	return BackupSettings{
		LocalEnabled:      true,
		LocalDir:          "/var/backups/netlynx",
		LocalRetainDays:   3,
		ShareKind:         "smb",
		ShareRetainDays:   3,
		SSHPort:           22,
		SSHTimeoutSeconds: 30,
		ScheduleHour:      2,
		ScheduleMinute:    0,
	}
}

func (s *Store) GetBackupSettings(ctx context.Context) (BackupSettings, error) {
	var r BackupSettings
	err := s.pool.QueryRow(ctx, `
		SELECT schedule_enabled, schedule_hour, schedule_minute,
		       local_enabled, local_dir, local_retain_days,
		       email_enabled, email_to,
		       share_enabled, share_kind, share_url, share_username, share_password, share_domain, share_retain_days,
		       switch_cfg_enabled, ssh_user, ssh_password, ssh_port, ssh_enable_password, ssh_timeout_seconds,
		       last_run_at, last_status, last_error, last_log
		FROM backup_settings WHERE id = 1`,
	).Scan(
		&r.ScheduleEnabled, &r.ScheduleHour, &r.ScheduleMinute,
		&r.LocalEnabled, &r.LocalDir, &r.LocalRetainDays,
		&r.EmailEnabled, &r.EmailTo,
		&r.ShareEnabled, &r.ShareKind, &r.ShareURL, &r.ShareUsername, &r.SharePassword, &r.ShareDomain, &r.ShareRetainDays,
		&r.SwitchCfgEnabled, &r.SSHUser, &r.SSHPassword, &r.SSHPort, &r.SSHEnablePassword, &r.SSHTimeoutSeconds,
		&r.LastRunAt, &r.LastStatus, &r.LastError, &r.LastLog,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return defaultBackupSettings(), nil
	}
	if err != nil {
		return r, err
	}
	return normalizeBackupSettings(r), nil
}

func normalizeBackupSettings(r BackupSettings) BackupSettings {
	if r.LocalDir == "" {
		r.LocalDir = "/var/backups/netlynx"
	}
	if r.LocalRetainDays < 1 {
		r.LocalRetainDays = 3
	}
	if r.LocalRetainDays > 365 {
		r.LocalRetainDays = 365
	}
	if r.ShareRetainDays < 1 {
		r.ShareRetainDays = 3
	}
	if r.ShareRetainDays > 365 {
		r.ShareRetainDays = 365
	}
	if r.ShareKind != "nfs" {
		r.ShareKind = "smb"
	}
	if r.SSHPort <= 0 {
		r.SSHPort = 22
	}
	if r.SSHTimeoutSeconds < 5 {
		r.SSHTimeoutSeconds = 30
	}
	if r.ScheduleHour < 0 || r.ScheduleHour > 23 {
		r.ScheduleHour = 2
	}
	if r.ScheduleMinute < 0 || r.ScheduleMinute > 59 {
		r.ScheduleMinute = 0
	}
	return r
}

func (s *Store) UpsertBackupSettings(ctx context.Context, in BackupSettings) error {
	in = normalizeBackupSettings(in)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO backup_settings (
			id, schedule_enabled, schedule_hour, schedule_minute,
			local_enabled, local_dir, local_retain_days,
			email_enabled, email_to,
			share_enabled, share_kind, share_url, share_username, share_password, share_domain, share_retain_days,
			switch_cfg_enabled, ssh_user, ssh_password, ssh_port, ssh_enable_password, ssh_timeout_seconds,
			updated_at
		) VALUES (
			1, $1, $2, $3,
			$4, $5, $6,
			$7, $8,
			$9, $10, $11, $12, $13, $14, $15,
			$16, $17, $18, $19, $20, $21,
			now()
		)
		ON CONFLICT (id) DO UPDATE SET
			schedule_enabled = EXCLUDED.schedule_enabled,
			schedule_hour = EXCLUDED.schedule_hour,
			schedule_minute = EXCLUDED.schedule_minute,
			local_enabled = EXCLUDED.local_enabled,
			local_dir = EXCLUDED.local_dir,
			local_retain_days = EXCLUDED.local_retain_days,
			email_enabled = EXCLUDED.email_enabled,
			email_to = EXCLUDED.email_to,
			share_enabled = EXCLUDED.share_enabled,
			share_kind = EXCLUDED.share_kind,
			share_url = EXCLUDED.share_url,
			share_username = EXCLUDED.share_username,
			share_password = EXCLUDED.share_password,
			share_domain = EXCLUDED.share_domain,
			share_retain_days = EXCLUDED.share_retain_days,
			switch_cfg_enabled = EXCLUDED.switch_cfg_enabled,
			ssh_user = EXCLUDED.ssh_user,
			ssh_password = EXCLUDED.ssh_password,
			ssh_port = EXCLUDED.ssh_port,
			ssh_enable_password = EXCLUDED.ssh_enable_password,
			ssh_timeout_seconds = EXCLUDED.ssh_timeout_seconds,
			updated_at = now()`,
		in.ScheduleEnabled, in.ScheduleHour, in.ScheduleMinute,
		in.LocalEnabled, in.LocalDir, in.LocalRetainDays,
		in.EmailEnabled, in.EmailTo,
		in.ShareEnabled, in.ShareKind, in.ShareURL, in.ShareUsername, in.SharePassword, in.ShareDomain, in.ShareRetainDays,
		in.SwitchCfgEnabled, in.SSHUser, in.SSHPassword, in.SSHPort, in.SSHEnablePassword, in.SSHTimeoutSeconds,
	)
	return err
}

func (s *Store) SetBackupRunResult(ctx context.Context, status, errMsg, logText string) error {
	status = strings.TrimSpace(status)
	if status == "" {
		status = "fail"
	}
	var errVal interface{}
	if strings.TrimSpace(errMsg) == "" {
		errVal = nil
	} else {
		errVal = errMsg
	}
	var logVal interface{}
	if strings.TrimSpace(logText) == "" {
		logVal = nil
	} else {
		logVal = logText
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE backup_settings SET last_run_at = now(), last_status = $1, last_error = $2, last_log = $3, updated_at = now()
		WHERE id = 1`, status, errVal, logVal)
	return err
}

func (s *Store) SetBackupLog(ctx context.Context, logText string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE backup_settings SET last_log = $1, updated_at = now()
		WHERE id = 1`, logText)
	return err
}

type DeviceSSHInput struct {
	SSHUser           *string
	SSHPassword       *string // nil = не менять; "" = очистить
	SSHPort           *int
	SSHEnablePassword *string // nil = не менять; "" = очистить
	SSHVendor         string
}

func NormalizeSSHVendor(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "ubiquiti", "eltex", "snr":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return "auto"
	}
}

func (s *Store) UpdateDeviceSSH(ctx context.Context, id int64, in DeviceSSHInput) error {
	vendor := NormalizeSSHVendor(in.SSHVendor)
	var port interface{}
	if in.SSHPort != nil {
		p := *in.SSHPort
		if p <= 0 {
			port = nil
		} else if p > 65535 {
			return fmt.Errorf("ssh_port: 1–65535")
		} else {
			port = p
		}
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE devices SET
			ssh_user = CASE WHEN $2::boolean THEN NULLIF(btrim($3::text), '') ELSE ssh_user END,
			ssh_password = CASE
				WHEN $4::boolean THEN NULLIF(btrim($5::text), '')
				ELSE ssh_password
			END,
			ssh_port = CASE WHEN $6::boolean THEN $7::int ELSE ssh_port END,
			ssh_enable_password = CASE
				WHEN $8::boolean THEN NULLIF(btrim($9::text), '')
				ELSE ssh_enable_password
			END,
			ssh_vendor = CASE WHEN $10::boolean THEN $11 ELSE ssh_vendor END,
			updated_at = now()
		WHERE id = $1`,
		id,
		in.SSHUser != nil, derefStr(in.SSHUser),
		in.SSHPassword != nil, derefStr(in.SSHPassword),
		in.SSHPort != nil, port,
		in.SSHEnablePassword != nil, derefStr(in.SSHEnablePassword),
		in.SSHVendor != "", vendor,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrDeviceNotFound
	}
	return nil
}

