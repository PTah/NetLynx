package store

import (
	"context"
	"errors"
	"fmt"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/secrets"
	"github.com/jackc/pgx/v5"
)

// SecretsPlaintextStats — сколько значений ещё без enc:v1: в scope-колонках.
type SecretsPlaintextStats struct {
	DevicesCommunity     int `json:"devices_community"`
	DevicesV3AuthPass    int `json:"devices_v3_auth_pass"`
	DevicesV3PrivPass    int `json:"devices_v3_priv_pass"`
	DevicesSSHPassword   int `json:"devices_ssh_password"`
	DevicesSSHEnablePass int `json:"devices_ssh_enable_password"`
	SMTPPassword         int `json:"smtp_password"`
	TelegramBotToken     int `json:"telegram_bot_token"`
	UISPAPIToken         int `json:"uisp_api_token"`
	BackupSharePassword  int `json:"backup_share_password"`
	BackupSSHPassword    int `json:"backup_ssh_password"`
	BackupSSHEnablePass  int `json:"backup_ssh_enable_password"`
	Total                int `json:"total"`
}

func (s *Store) countPlaintextCol(ctx context.Context, table, col string) (int, error) {
	q := fmt.Sprintf(`
		SELECT COUNT(*) FROM %s
		WHERE %s IS NOT NULL AND btrim(%s) <> ''
		  AND %s NOT LIKE 'enc:v1:%%'`, table, col, col, col)
	var n int
	err := s.pool.QueryRow(ctx, q).Scan(&n)
	return n, err
}

// CountPlaintextSecrets считает незашифрованные значения в scope.
func (s *Store) CountPlaintextSecrets(ctx context.Context) (SecretsPlaintextStats, error) {
	var st SecretsPlaintextStats
	type item struct {
		table, col string
		dst        *int
	}
	items := []item{
		{"devices", "community", &st.DevicesCommunity},
		{"devices", "v3_auth_pass", &st.DevicesV3AuthPass},
		{"devices", "v3_priv_pass", &st.DevicesV3PrivPass},
		{"devices", "ssh_password", &st.DevicesSSHPassword},
		{"devices", "ssh_enable_password", &st.DevicesSSHEnablePass},
		{"notification_settings", "smtp_password", &st.SMTPPassword},
		{"notification_settings", "telegram_bot_token", &st.TelegramBotToken},
		{"uisp_settings", "api_token", &st.UISPAPIToken},
		{"backup_settings", "share_password", &st.BackupSharePassword},
		{"backup_settings", "ssh_password", &st.BackupSSHPassword},
		{"backup_settings", "ssh_enable_password", &st.BackupSSHEnablePass},
	}
	for _, it := range items {
		n, e := s.countPlaintextCol(ctx, it.table, it.col)
		if e != nil {
			return st, e
		}
		*it.dst = n
		st.Total += n
	}
	return st, nil
}

// SecretsRewrapResult — итог идемпотентного plaintext→enc:v1.
type SecretsRewrapResult struct {
	Rewrapped int                   `json:"rewrapped"`
	Skipped   int                   `json:"skipped"`
	Before    SecretsPlaintextStats `json:"before"`
	After     SecretsPlaintextStats `json:"after"`
}

// RewrapAllSecrets шифрует все plaintext scope-поля. Требует включённый SecretsBox.
func (s *Store) RewrapAllSecrets(ctx context.Context) (SecretsRewrapResult, error) {
	var out SecretsRewrapResult
	b := s.secretsBox()
	if !b.Enabled() {
		return out, secrets.ErrNoKey
	}
	before, err := s.CountPlaintextSecrets(ctx)
	if err != nil {
		return out, err
	}
	out.Before = before

	n, sk, err := s.rewrapDevices(ctx, b)
	out.Rewrapped += n
	out.Skipped += sk
	if err != nil {
		return out, err
	}
	n, sk, err = s.rewrapNotification(ctx, b)
	out.Rewrapped += n
	out.Skipped += sk
	if err != nil {
		return out, err
	}
	n, sk, err = s.rewrapUISP(ctx, b)
	out.Rewrapped += n
	out.Skipped += sk
	if err != nil {
		return out, err
	}
	n, sk, err = s.rewrapBackup(ctx, b)
	out.Rewrapped += n
	out.Skipped += sk
	if err != nil {
		return out, err
	}

	after, err := s.CountPlaintextSecrets(ctx)
	if err != nil {
		return out, err
	}
	out.After = after
	return out, nil
}

func rewrapCell(b *secrets.Box, raw *string) (newVal *string, changed, skipped bool, err error) {
	if raw == nil {
		return nil, false, true, nil
	}
	v := *raw
	if v == "" {
		return raw, false, true, nil
	}
	if secrets.IsEncrypted(v) {
		return raw, false, true, nil
	}
	enc, err := b.Seal(v)
	if err != nil {
		return nil, false, false, err
	}
	return &enc, true, false, nil
}

func (s *Store) rewrapDevices(ctx context.Context, b *secrets.Box) (rewrapped, skipped int, err error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, community, v3_auth_pass, v3_priv_pass, ssh_password, ssh_enable_password
		FROM devices`)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()
	type row struct {
		id               int64
		c, a, p, ssh, en *string
	}
	var list []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.c, &r.a, &r.p, &r.ssh, &r.en); err != nil {
			return rewrapped, skipped, err
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return rewrapped, skipped, err
	}
	for _, r := range list {
		c, ch, sk, e := rewrapCell(b, r.c)
		if e != nil {
			return rewrapped, skipped, e
		}
		if sk {
			skipped++
		}
		if ch {
			rewrapped++
		}
		a, ch, sk, e := rewrapCell(b, r.a)
		if e != nil {
			return rewrapped, skipped, e
		}
		if sk {
			skipped++
		}
		if ch {
			rewrapped++
		}
		p, ch, sk, e := rewrapCell(b, r.p)
		if e != nil {
			return rewrapped, skipped, e
		}
		if sk {
			skipped++
		}
		if ch {
			rewrapped++
		}
		ssh, ch, sk, e := rewrapCell(b, r.ssh)
		if e != nil {
			return rewrapped, skipped, e
		}
		if sk {
			skipped++
		}
		if ch {
			rewrapped++
		}
		en, ch, sk, e := rewrapCell(b, r.en)
		if e != nil {
			return rewrapped, skipped, e
		}
		if sk {
			skipped++
		}
		if ch {
			rewrapped++
		}
		if _, err := s.pool.Exec(ctx, `
			UPDATE devices SET
				community = $2, v3_auth_pass = $3, v3_priv_pass = $4,
				ssh_password = $5, ssh_enable_password = $6, updated_at = now()
			WHERE id = $1`, r.id, c, a, p, ssh, en); err != nil {
			return rewrapped, skipped, err
		}
	}
	return rewrapped, skipped, nil
}

func (s *Store) rewrapNotification(ctx context.Context, b *secrets.Box) (rewrapped, skipped int, err error) {
	var smtp, tg *string
	err = s.pool.QueryRow(ctx, `
		SELECT smtp_password, telegram_bot_token FROM notification_settings WHERE id = 1`,
	).Scan(&smtp, &tg)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}
	smtp2, ch, sk, e := rewrapCell(b, smtp)
	if e != nil {
		return 0, 0, e
	}
	if sk {
		skipped++
	}
	if ch {
		rewrapped++
	}
	tg2, ch, sk, e := rewrapCell(b, tg)
	if e != nil {
		return rewrapped, skipped, e
	}
	if sk {
		skipped++
	}
	if ch {
		rewrapped++
	}
	_, err = s.pool.Exec(ctx, `
		UPDATE notification_settings SET smtp_password = $1, telegram_bot_token = $2, updated_at = now()
		WHERE id = 1`, smtp2, tg2)
	return rewrapped, skipped, err
}

func (s *Store) rewrapUISP(ctx context.Context, b *secrets.Box) (rewrapped, skipped int, err error) {
	var tok *string
	err = s.pool.QueryRow(ctx, `SELECT api_token FROM uisp_settings WHERE id = 1`).Scan(&tok)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}
	tok2, ch, sk, e := rewrapCell(b, tok)
	if e != nil {
		return 0, 0, e
	}
	if sk {
		skipped++
	}
	if ch {
		rewrapped++
	}
	_, err = s.pool.Exec(ctx, `UPDATE uisp_settings SET api_token = $1, updated_at = now() WHERE id = 1`, tok2)
	return rewrapped, skipped, err
}

func (s *Store) rewrapBackup(ctx context.Context, b *secrets.Box) (rewrapped, skipped int, err error) {
	var share, ssh, en *string
	err = s.pool.QueryRow(ctx, `
		SELECT share_password, ssh_password, ssh_enable_password FROM backup_settings WHERE id = 1`,
	).Scan(&share, &ssh, &en)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}
	share2, ch, sk, e := rewrapCell(b, share)
	if e != nil {
		return 0, 0, e
	}
	if sk {
		skipped++
	}
	if ch {
		rewrapped++
	}
	ssh2, ch, sk, e := rewrapCell(b, ssh)
	if e != nil {
		return rewrapped, skipped, e
	}
	if sk {
		skipped++
	}
	if ch {
		rewrapped++
	}
	en2, ch, sk, e := rewrapCell(b, en)
	if e != nil {
		return rewrapped, skipped, e
	}
	if sk {
		skipped++
	}
	if ch {
		rewrapped++
	}
	_, err = s.pool.Exec(ctx, `
		UPDATE backup_settings SET
			share_password = $1, ssh_password = $2, ssh_enable_password = $3, updated_at = now()
		WHERE id = 1`, share2, ssh2, en2)
	return rewrapped, skipped, err
}
