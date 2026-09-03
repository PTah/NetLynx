package store

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const snmpTrapLogRetain = 2000

// Режимы мгновенных LINK-событий из trap.
const (
	LinkTrapEventsOff       = "off"
	LinkTrapEventsPerDevice = "per_device"
	LinkTrapEventsAll       = "all"

	LinkTrapEffectsNotify = "notify"
	LinkTrapEffectsFull   = "full"
)

// NormalizeLinkTrapEventsMode: off | per_device | all.
func NormalizeLinkTrapEventsMode(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case LinkTrapEventsPerDevice:
		return LinkTrapEventsPerDevice
	case LinkTrapEventsAll:
		return LinkTrapEventsAll
	default:
		return LinkTrapEventsOff
	}
}

// NormalizeLinkTrapEffects: notify | full.
func NormalizeLinkTrapEffects(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case LinkTrapEffectsFull:
		return LinkTrapEffectsFull
	default:
		return LinkTrapEffectsNotify
	}
}

// AllowLinkTrapEvents — gate для мгновенного LINK_* из trap.
func AllowLinkTrapEvents(mode string, deviceFound, trustLinkTraps bool) bool {
	switch NormalizeLinkTrapEventsMode(mode) {
	case LinkTrapEventsAll:
		return deviceFound
	case LinkTrapEventsPerDevice:
		return deviceFound && trustLinkTraps
	default:
		return false
	}
}

// SNMPTrapSettings — глобальные настройки приёма/логирования traps (id=1).
type SNMPTrapSettings struct {
	LogEnabled          bool      `json:"log_enabled"`
	ListenEnabled       bool      `json:"listen_enabled"`
	ListenPort          int       `json:"listen_port"`
	TrapIncludeLabels   string    `json:"trap_include_labels,omitempty"`
	LinkTrapEventsMode  string    `json:"link_trap_events_mode"`
	LinkTrapEffects     string    `json:"link_trap_effects"`
	UpdatedAt           time.Time `json:"updated_at,omitempty"`
}

// SNMPTrapLogRow — одна запись тестового журнала traps.
type SNMPTrapLogRow struct {
	ID          int64                  `json:"id"`
	ReceivedAt  time.Time              `json:"received_at"`
	SourceIP    string                 `json:"source_ip"`
	DeviceID    *int64                 `json:"device_id,omitempty"`
	SNMPVersion string                 `json:"snmp_version,omitempty"`
	Community   string                 `json:"community,omitempty"`
	TrapOID     string                 `json:"trap_oid,omitempty"`
	IfIndex     *int                   `json:"if_index,omitempty"`
	Payload     map[string]interface{} `json:"payload"`
}

// SNMPTrapPendingLink — ожидание подтверждения linkUp/linkDown опросом.
type SNMPTrapPendingLink struct {
	DeviceID     int64
	IfIndex      int
	ExpectedOper int // 1=up, 2=down
	TrapLabel    string
	SourceIP     string
	ReceivedAt   time.Time
}

func (s *Store) GetSNMPTrapSettings(ctx context.Context) (SNMPTrapSettings, error) {
	var out SNMPTrapSettings
	var labels *string
	var mode, effects string
	err := s.pool.QueryRow(ctx, `
		SELECT log_enabled, listen_enabled, listen_port, trap_include_labels,
			COALESCE(link_trap_events_mode, 'off'), COALESCE(link_trap_effects, 'notify'), updated_at
		FROM snmp_trap_settings WHERE id = 1`,
	).Scan(&out.LogEnabled, &out.ListenEnabled, &out.ListenPort, &labels, &mode, &effects, &out.UpdatedAt)
	if err == pgx.ErrNoRows {
		return SNMPTrapSettings{
			ListenEnabled:      true,
			ListenPort:         9162,
			LinkTrapEventsMode: LinkTrapEventsOff,
			LinkTrapEffects:    LinkTrapEffectsNotify,
		}, nil
	}
	if labels != nil {
		out.TrapIncludeLabels = *labels
	}
	if out.ListenPort < 1 || out.ListenPort > 65535 {
		out.ListenPort = 9162
	}
	out.LinkTrapEventsMode = NormalizeLinkTrapEventsMode(mode)
	out.LinkTrapEffects = NormalizeLinkTrapEffects(effects)
	return out, err
}

type PatchSNMPTrapSettingsInput struct {
	LogEnabled         *bool
	ListenEnabled      *bool
	ListenPort         *int
	TrapIncludeLabels  *string
	LinkTrapEventsMode *string
	LinkTrapEffects    *string
}

func (s *Store) PatchSNMPTrapSettings(ctx context.Context, in PatchSNMPTrapSettingsInput) error {
	cur, err := s.GetSNMPTrapSettings(ctx)
	if err != nil {
		return err
	}
	le := cur.LogEnabled
	if in.LogEnabled != nil {
		le = *in.LogEnabled
	}
	listenEn := cur.ListenEnabled
	if in.ListenEnabled != nil {
		listenEn = *in.ListenEnabled
	}
	port := cur.ListenPort
	if in.ListenPort != nil {
		port = *in.ListenPort
	}
	if port < 1 || port > 65535 {
		port = 9162
	}
	labels := cur.TrapIncludeLabels
	if in.TrapIncludeLabels != nil {
		labels = *in.TrapIncludeLabels
	}
	mode := cur.LinkTrapEventsMode
	if in.LinkTrapEventsMode != nil {
		mode = NormalizeLinkTrapEventsMode(*in.LinkTrapEventsMode)
	}
	effects := cur.LinkTrapEffects
	if in.LinkTrapEffects != nil {
		effects = NormalizeLinkTrapEffects(*in.LinkTrapEffects)
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO snmp_trap_settings (
			id, log_enabled, listen_enabled, listen_port, trap_include_labels,
			link_trap_events_mode, link_trap_effects, updated_at
		)
		VALUES (1, $1, $2, $3, NULLIF($4, ''), $5, $6, now())
		ON CONFLICT (id) DO UPDATE SET
			log_enabled = EXCLUDED.log_enabled,
			listen_enabled = EXCLUDED.listen_enabled,
			listen_port = EXCLUDED.listen_port,
			trap_include_labels = EXCLUDED.trap_include_labels,
			link_trap_events_mode = EXCLUDED.link_trap_events_mode,
			link_trap_effects = EXCLUDED.link_trap_effects,
			updated_at = now()`,
		le, listenEn, port, labels, mode, effects,
	)
	return err
}

func (s *Store) SetSNMPTrapLogEnabled(ctx context.Context, enabled bool) error {
	return s.PatchSNMPTrapSettings(ctx, PatchSNMPTrapSettingsInput{LogEnabled: &enabled})
}

func (s *Store) InsertSNMPTrapLog(
	ctx context.Context,
	sourceIP string,
	deviceID *int64,
	snmpVersion, community, trapOID string,
	ifIndex *int,
	payload map[string]interface{},
) (int64, error) {
	if payload == nil {
		payload = map[string]interface{}{}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	var id int64
	err = s.pool.QueryRow(ctx, `
		INSERT INTO snmp_trap_logs (
			source_ip, device_id, snmp_version, community, trap_oid, if_index, payload
		) VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''), $6, $7::jsonb)
		RETURNING id`,
		sourceIP, deviceID, snmpVersion, community, trapOID, ifIndex, string(raw),
	).Scan(&id)
	if err != nil {
		return 0, err
	}
	_, _ = s.pool.Exec(ctx, `
		DELETE FROM snmp_trap_logs
		WHERE id IN (
			SELECT id FROM snmp_trap_logs
			ORDER BY id DESC
			OFFSET $1
		)`, snmpTrapLogRetain)
	return id, nil
}

func (s *Store) ListSNMPTrapLogs(ctx context.Context, limit int) ([]SNMPTrapLogRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, received_at, source_ip, device_id, COALESCE(snmp_version, ''),
			COALESCE(community, ''), COALESCE(trap_oid, ''), if_index, payload
		FROM snmp_trap_logs
		ORDER BY id DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]SNMPTrapLogRow, 0, limit)
	for rows.Next() {
		var r SNMPTrapLogRow
		var raw []byte
		if err := rows.Scan(
			&r.ID, &r.ReceivedAt, &r.SourceIP, &r.DeviceID, &r.SNMPVersion,
			&r.Community, &r.TrapOID, &r.IfIndex, &raw,
		); err != nil {
			return nil, err
		}
		r.Payload = map[string]interface{}{}
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &r.Payload)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) ClearSNMPTrapLogs(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM snmp_trap_logs`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *Store) CountSNMPTrapLogs(ctx context.Context) (int64, error) {
	var n int64
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM snmp_trap_logs`).Scan(&n)
	return n, err
}

// UpsertTrapPendingLink запоминает linkUp/linkDown до подтверждения опросом.
func (s *Store) UpsertTrapPendingLink(ctx context.Context, deviceID int64, ifIndex, expectedOper int, label, sourceIP string) error {
	if deviceID <= 0 || ifIndex <= 0 || (expectedOper != 1 && expectedOper != 2) {
		return nil
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO snmp_trap_pending_link (device_id, if_index, expected_oper, trap_label, source_ip, received_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), now())
		ON CONFLICT (device_id, if_index) DO UPDATE SET
			expected_oper = EXCLUDED.expected_oper,
			trap_label = EXCLUDED.trap_label,
			source_ip = EXCLUDED.source_ip,
			received_at = now()`,
		deviceID, ifIndex, expectedOper, label, sourceIP,
	)
	return err
}

// TakeTrapPendingLink удаляет pending, если expected_oper совпал и запись свежая.
// Возвращает pending и true, если подтверждение trap'ом ожидалось.
func (s *Store) TakeTrapPendingLink(ctx context.Context, deviceID int64, ifIndex, expectedOper int) (*SNMPTrapPendingLink, bool, error) {
	_, _ = s.pool.Exec(ctx, `
		DELETE FROM snmp_trap_pending_link WHERE received_at < now() - interval '15 minutes'`)
	var p SNMPTrapPendingLink
	var label, src *string
	err := s.pool.QueryRow(ctx, `
		DELETE FROM snmp_trap_pending_link
		WHERE device_id = $1 AND if_index = $2 AND expected_oper = $3
			AND received_at >= now() - interval '15 minutes'
		RETURNING device_id, if_index, expected_oper, trap_label, source_ip, received_at`,
		deviceID, ifIndex, expectedOper,
	).Scan(&p.DeviceID, &p.IfIndex, &p.ExpectedOper, &label, &src, &p.ReceivedAt)
	if err == pgx.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if label != nil {
		p.TrapLabel = *label
	}
	if src != nil {
		p.SourceIP = *src
	}
	return &p, true, nil
}

// ConfirmRecentTrapLinkEvent помечает недавнее LINK_* от trap как подтверждённое опросом.
// Возвращает true, если такая запись найдена (дубликат от poller не нужен).
func (s *Store) ConfirmRecentTrapLinkEvent(ctx context.Context, deviceID int64, ifIndex int, eventType string) (bool, error) {
	if deviceID <= 0 || ifIndex <= 0 || (eventType != "LINK_UP" && eventType != "LINK_DOWN") {
		return false, nil
	}
	var id int64
	err := s.pool.QueryRow(ctx, `
		UPDATE events
		SET payload = COALESCE(payload, '{}'::jsonb) || jsonb_build_object('trap_confirmed', true)
		WHERE id = (
			SELECT id FROM events
			WHERE device_id = $1 AND if_index = $2 AND event_type = $3
				AND COALESCE(payload->>'source', '') = 'trap'
				AND created_at >= now() - interval '15 minutes'
			ORDER BY id DESC
			LIMIT 1
		)
		RETURNING id`,
		deviceID, ifIndex, eventType,
	).Scan(&id)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
