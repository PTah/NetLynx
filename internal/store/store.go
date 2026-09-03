package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// PollDevice — учётные данные и адрес для SNMP (не использовать в REST).
type PollDevice struct {
	ID                  int64
	Name                string
	Host                string
	SNMPVersion         string
	Community           *string
	V3User              *string
	V3AuthProtocol      *string
	V3AuthPass          *string
	V3PrivProtocol      *string
	V3PrivPass          *string
	V3EngineID          *string
	PollIntervalSeconds      int
	UtilHighPct              *float32
	UtilOkPct                *float32
	FDBPollIntervalSeconds   *int
	LastPollAt               *time.Time
	LastFDBPollAt            *time.Time
	FDBBaselineAt            *time.Time
}

func (s *Store) GetPollDevice(ctx context.Context, id int64) (*PollDevice, error) {
	var d PollDevice
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, COALESCE(host, '') AS host, snmp_version, community,
		       v3_user, v3_auth_protocol, v3_auth_pass, v3_priv_protocol, v3_priv_pass, v3_engine_id,
		       poll_interval_seconds, util_high_pct, util_ok_pct, fdb_poll_interval_seconds,
		       last_poll_at, last_fdb_poll_at, fdb_baseline_at
		FROM devices WHERE id = $1`, id,
	).Scan(
		&d.ID, &d.Name, &d.Host, &d.SNMPVersion, &d.Community,
		&d.V3User, &d.V3AuthProtocol, &d.V3AuthPass, &d.V3PrivProtocol, &d.V3PrivPass, &d.V3EngineID,
		&d.PollIntervalSeconds, &d.UtilHighPct, &d.UtilOkPct, &d.FDBPollIntervalSeconds,
		&d.LastPollAt, &d.LastFDBPollAt, &d.FDBBaselineAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (s *Store) FindDeviceIDByHost(ctx context.Context, host string) (int64, bool, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return 0, false, nil
	}
	var id int64
	err := s.pool.QueryRow(ctx, `SELECT id FROM devices WHERE host = $1 ORDER BY id LIMIT 1`, host).Scan(&id)
	if err == pgx.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}

func (s *Store) ListPollDevices(ctx context.Context) ([]PollDevice, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, COALESCE(host, '') AS host, snmp_version, community,
		       v3_user, v3_auth_protocol, v3_auth_pass, v3_priv_protocol, v3_priv_pass, v3_engine_id,
		       poll_interval_seconds, util_high_pct, util_ok_pct, fdb_poll_interval_seconds,
		       last_poll_at, last_fdb_poll_at, fdb_baseline_at
		FROM devices ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PollDevice
	for rows.Next() {
		var d PollDevice
		if err := rows.Scan(
			&d.ID, &d.Name, &d.Host, &d.SNMPVersion, &d.Community,
			&d.V3User, &d.V3AuthProtocol, &d.V3AuthPass, &d.V3PrivProtocol, &d.V3PrivPass, &d.V3EngineID,
			&d.PollIntervalSeconds, &d.UtilHighPct, &d.UtilOkPct, &d.FDBPollIntervalSeconds,
			&d.LastPollAt, &d.LastFDBPollAt, &d.FDBBaselineAt,
		); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) ListDevices(ctx context.Context) ([]models.Device, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, COALESCE(host, '') AS host, location, uisp_device_id, uisp_overview_status, snmp_version, community,
		       v3_user, v3_auth_protocol, v3_priv_protocol, v3_engine_id,
		       poll_interval_seconds, util_high_pct, util_ok_pct, fdb_poll_interval_seconds,
		       created_at, updated_at, last_poll_at, last_snmp_ok, last_snmp_error,
		       last_ping_ok, last_ping_at, last_ping_rtt_ms, online_override, offline_since,
		       sys_name, sys_descr, chassis_mac, cpu_profile, device_category, last_cpu_pct, last_cpu_at, last_sys_uptime_cs, fdb_monitoring_status,
		       ssh_user, ssh_password, ssh_port, ssh_enable_password, COALESCE(NULLIF(btrim(ssh_vendor), ''), 'auto'),
		       COALESCE(trust_link_traps, false)
		FROM devices ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Device
	for rows.Next() {
		var d models.Device
		if err := rows.Scan(
			&d.ID, &d.Name, &d.Host, &d.Location, &d.UISPDeviceID, &d.UISPOverviewStatus, &d.SNMPVersion, &d.Community,
			&d.V3User, &d.V3AuthProtocol, &d.V3PrivProtocol, &d.V3EngineID,
			&d.PollIntervalSeconds, &d.UtilHighPct, &d.UtilOkPct, &d.FDBPollIntervalSeconds,
			&d.CreatedAt, &d.UpdatedAt, &d.LastPollAt, &d.LastSNMPOK, &d.LastSNMPError,
			&d.LastPingOK, &d.LastPingAt, &d.LastPingRTTMs, &d.OnlineOverride, &d.OfflineSince,
			&d.SysName, &d.SysDescr, &d.ChassisMAC, &d.CPUProfile, &d.DeviceCategory, &d.LastCPUPct, &d.LastCPUAt, &d.LastSysUptimeCs, &d.FDBMonitoringStatus,
			&d.SSHUser, &d.SSHPassword, &d.SSHPort, &d.SSHEnablePassword, &d.SSHVendor,
			&d.TrustLinkTraps,
		); err != nil {
			return nil, err
		}
		if d.DeviceCategory == "" {
			d.DeviceCategory = DeviceCategorySwitch
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) CountDevices(ctx context.Context) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM devices`).Scan(&n)
	return n, err
}

func (s *Store) GetDevice(ctx context.Context, id int64) (*models.Device, error) {
	var d models.Device
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, COALESCE(host, '') AS host, location, uisp_device_id, uisp_overview_status, snmp_version, community,
		       v3_user, v3_auth_protocol, v3_priv_protocol, v3_engine_id,
		       poll_interval_seconds, util_high_pct, util_ok_pct, fdb_poll_interval_seconds,
		       created_at, updated_at, last_poll_at, last_snmp_ok, last_snmp_error,
		       last_ping_ok, last_ping_at, last_ping_rtt_ms, online_override, offline_since,
		       sys_name, sys_descr, chassis_mac, cpu_profile, device_category, last_cpu_pct, last_cpu_at, last_sys_uptime_cs, fdb_monitoring_status,
		       ssh_user, ssh_password, ssh_port, ssh_enable_password, COALESCE(NULLIF(btrim(ssh_vendor), ''), 'auto'),
		       COALESCE(trust_link_traps, false)
		FROM devices WHERE id = $1`, id).Scan(
		&d.ID, &d.Name, &d.Host, &d.Location, &d.UISPDeviceID, &d.UISPOverviewStatus, &d.SNMPVersion, &d.Community,
		&d.V3User, &d.V3AuthProtocol, &d.V3PrivProtocol, &d.V3EngineID,
		&d.PollIntervalSeconds, &d.UtilHighPct, &d.UtilOkPct, &d.FDBPollIntervalSeconds,
		&d.CreatedAt, &d.UpdatedAt, &d.LastPollAt, &d.LastSNMPOK, &d.LastSNMPError,
		&d.LastPingOK, &d.LastPingAt, &d.LastPingRTTMs, &d.OnlineOverride, &d.OfflineSince,
		&d.SysName, &d.SysDescr, &d.ChassisMAC, &d.CPUProfile, &d.DeviceCategory, &d.LastCPUPct, &d.LastCPUAt, &d.LastSysUptimeCs, &d.FDBMonitoringStatus,
		&d.SSHUser, &d.SSHPassword, &d.SSHPort, &d.SSHEnablePassword, &d.SSHVendor,
		&d.TrustLinkTraps,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if d.DeviceCategory == "" {
		d.DeviceCategory = DeviceCategorySwitch
	}
	return &d, nil
}

type CreateDeviceInput struct {
	Name                string
	Host                string
	Location            *string
	UISPDeviceID        *string
	DeviceCategory      string
	SNMPVersion         string
	Community           *string
	V3User              *string
	V3AuthProtocol      *string
	V3AuthPass          *string
	V3PrivProtocol      *string
	V3PrivPass          *string
	V3EngineID          *string
	PollIntervalSeconds int
	ChassisMAC          *string // опционально при promote из LLDP
}

func (s *Store) CreateDevice(ctx context.Context, in CreateDeviceInput) (int64, error) {
	if in.PollIntervalSeconds == 0 {
		in.PollIntervalSeconds = 60
	}
	in.Host = strings.TrimSpace(in.Host)
	// Пустой host допустим (узел без адреса: другой офис / склад / только LLDP).
	cat := NormalizeDeviceCategory(in.DeviceCategory)
	ok, err := s.DeviceCategoryExists(ctx, cat)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, fmt.Errorf("неизвестный тип узла: %s", cat)
	}
	var chassis *string
	if in.ChassisMAC != nil {
		if mac, ok := NormalizeMACQuery(*in.ChassisMAC); ok {
			chassis = &mac
		}
	}
	if in.Host == "" && chassis == nil {
		return 0, errors.New("нужен host или chassis MAC")
	}
	if err := s.CheckDeviceIdentity(ctx, in.Host, chassis, 0); err != nil {
		return 0, err
	}
	var hostVal interface{}
	if in.Host == "" {
		hostVal = nil
	} else {
		hostVal = in.Host
	}
	var id int64
	err = s.pool.QueryRow(ctx, `
		INSERT INTO devices (name, host, location, uisp_device_id, device_category, snmp_version, community,
			v3_user, v3_auth_protocol, v3_auth_pass, v3_priv_protocol, v3_priv_pass, v3_engine_id,
			poll_interval_seconds, chassis_mac)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		RETURNING id`,
		in.Name, hostVal, in.Location, in.UISPDeviceID, cat, in.SNMPVersion, in.Community,
		in.V3User, in.V3AuthProtocol, in.V3AuthPass, in.V3PrivProtocol, in.V3PrivPass, in.V3EngineID,
		in.PollIntervalSeconds, chassis,
	).Scan(&id)
	if err != nil {
		return 0, mapUniqueViolation(err)
	}
	return id, nil
}

func (s *Store) UpdateDevice(ctx context.Context, id int64, in CreateDeviceInput) error {
	if in.PollIntervalSeconds == 0 {
		in.PollIntervalSeconds = 60
	}
	in.Host = strings.TrimSpace(in.Host)
	if in.Host != "" {
		if err := s.CheckDeviceIdentity(ctx, in.Host, nil, id); err != nil {
			return err
		}
	}
	// Пустой host → NULL через NULLIF; ::text нужен, иначе PG не выводит тип параметра (42P08).
	tag, err := s.pool.Exec(ctx, `
		UPDATE devices SET
			name = $2,
			host = NULLIF(btrim($3::text), ''),
			snmp_version = $4,
			community = $5,
			v3_user = $6,
			v3_auth_protocol = $7,
			v3_auth_pass = $8,
			v3_priv_protocol = $9,
			v3_priv_pass = $10,
			v3_engine_id = $11,
			poll_interval_seconds = $12,
			last_ping_ok = CASE WHEN NULLIF(btrim($3::text), '') IS NULL THEN NULL ELSE last_ping_ok END,
			last_ping_at = CASE WHEN NULLIF(btrim($3::text), '') IS NULL THEN NULL ELSE last_ping_at END,
			last_ping_rtt_ms = CASE WHEN NULLIF(btrim($3::text), '') IS NULL THEN NULL ELSE last_ping_rtt_ms END,
			updated_at = now()
		WHERE id = $1`,
		id,
		in.Name, in.Host, in.SNMPVersion, in.Community,
		in.V3User, in.V3AuthProtocol, in.V3AuthPass, in.V3PrivProtocol, in.V3PrivPass, in.V3EngineID,
		in.PollIntervalSeconds,
	)
	if err != nil {
		return mapUniqueViolation(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrDeviceNotFound
	}
	return nil
}

func (s *Store) UpdateDevicePollMeta(ctx context.Context, id int64, ok bool, errMsg *string, sysName, sysDescr, cpuProfile *string, cpuPct *float32, chassisMAC *string, sysUptimeCs *int64) error {
	if chassisMAC != nil {
		if err := s.CheckDeviceIdentity(ctx, "", chassisMAC, id); err != nil {
			chassisMAC = nil // не затираем чужой MAC при опросе
		}
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE devices SET
			last_poll_at = now(),
			last_snmp_ok = $2,
			last_snmp_error = $3,
			sys_name = COALESCE($4, sys_name),
			sys_descr = COALESCE($5, sys_descr),
			cpu_profile = COALESCE($6, cpu_profile),
			last_cpu_pct = COALESCE($7, last_cpu_pct),
			last_cpu_at = CASE WHEN $7 IS NULL THEN last_cpu_at ELSE now() END,
			chassis_mac = COALESCE($8, chassis_mac),
			last_sys_uptime_cs = COALESCE($9, last_sys_uptime_cs),
			updated_at = now()
		WHERE id = $1`,
		id, ok, errMsg, sysName, sysDescr, cpuProfile, cpuPct, chassisMAC, sysUptimeCs,
	)
	return mapUniqueViolation(err)
}

func (s *Store) UpdateDevicePing(ctx context.Context, id int64, ok bool, rttMs *int) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE devices SET
			last_ping_ok = $2,
			last_ping_at = now(),
			last_ping_rtt_ms = $3,
			updated_at = now()
		WHERE id = $1`,
		id, ok, rttMs,
	)
	return err
}

// RefreshOfflineSince ставит offline_since при переходе в оффлайн и сбрасывает при возврате онлайн.
func (s *Store) RefreshOfflineSince(ctx context.Context, id int64) error {
	d, err := s.GetDevice(ctx, id)
	if err != nil {
		return err
	}
	if d == nil {
		return nil
	}
	if d.IsOnline() {
		_, err = s.pool.Exec(ctx, `UPDATE devices SET offline_since = NULL WHERE id = $1 AND offline_since IS NOT NULL`, id)
		return err
	}
	_, err = s.pool.Exec(ctx, `UPDATE devices SET offline_since = COALESCE(offline_since, now()) WHERE id = $1`, id)
	return err
}

func (s *Store) RefreshAllOfflineSince(ctx context.Context) error {
	rows, err := s.pool.Query(ctx, `SELECT id FROM devices`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		if err := s.RefreshOfflineSince(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) GetFDBSnapshot(ctx context.Context, deviceID int64) (map[string]int, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT mac, if_index
		FROM device_fdb_entries
		WHERE device_id = $1`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var mac string
		var ifIndex int
		if err := rows.Scan(&mac, &ifIndex); err != nil {
			return nil, err
		}
		out[mac] = ifIndex
	}
	return out, rows.Err()
}

// PortAccessSecurity — «память» MAC на access-порту (см. poller FDB).
type PortAccessSecurity struct {
	IfIndex    int
	BoundMAC   *string
	EmptySince *time.Time
}

func (s *Store) GetPortAccessSecurity(ctx context.Context, deviceID int64) (map[int]PortAccessSecurity, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT if_index, bound_mac, empty_since
		FROM port_access_security WHERE device_id = $1`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int]PortAccessSecurity)
	for rows.Next() {
		var r PortAccessSecurity
		if err := rows.Scan(&r.IfIndex, &r.BoundMAC, &r.EmptySince); err != nil {
			return nil, err
		}
		out[r.IfIndex] = r
	}
	return out, rows.Err()
}

func (s *Store) UpsertPortAccessSecurity(ctx context.Context, deviceID int64, ifIndex int, boundMAC *string, emptySince *time.Time) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO port_access_security (device_id, if_index, bound_mac, empty_since, updated_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (device_id, if_index) DO UPDATE SET
			bound_mac = EXCLUDED.bound_mac,
			empty_since = EXCLUDED.empty_since,
			updated_at = now()`,
		deviceID, ifIndex, boundMAC, emptySince)
	return err
}

// InterfaceSnapshot — строка из БД до опроса (для детектора линка и счётчиков).
type InterfaceSnapshot struct {
	IfIndex          int
	IfName           *string
	IfDescr          *string
	OperStatus       *int
	AdminStatus      *int
	HCInOctets       *int64
	HCOutOctets      *int64
	CountersPolledAt *time.Time
	IfSpeed          *int64
	IfHighSpeed      *int64
	UtilHighPct      *float32
	UtilOkPct        *float32
	UtilHighActive   bool
	DescrOverride    *string
	PortRole         string
	CLIPortMode      *string
}

// DisplayDescr — подпись для UI/событий: ручная, иначе SNMP ifDescr/ifAlias.
func (s InterfaceSnapshot) DisplayDescr() string {
	if s.DescrOverride != nil {
		if t := strings.TrimSpace(*s.DescrOverride); t != "" {
			return t
		}
	}
	if s.IfDescr != nil {
		return strings.TrimSpace(*s.IfDescr)
	}
	return ""
}

func (s *Store) ListInterfaceSnapshots(ctx context.Context, deviceID int64) (map[int]InterfaceSnapshot, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT if_index, if_name, if_descr, oper_status, admin_status, hc_in_octets, hc_out_octets, counters_polled_at,
		       if_speed, if_high_speed, util_high_pct, util_ok_pct, util_high_active, descr_override,
		       COALESCE(port_role, ''), cli_port_mode
		FROM device_interfaces WHERE device_id = $1`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := make(map[int]InterfaceSnapshot)
	for rows.Next() {
		var r InterfaceSnapshot
		if err := rows.Scan(&r.IfIndex, &r.IfName, &r.IfDescr, &r.OperStatus, &r.AdminStatus, &r.HCInOctets, &r.HCOutOctets, &r.CountersPolledAt,
			&r.IfSpeed, &r.IfHighSpeed, &r.UtilHighPct, &r.UtilOkPct, &r.UtilHighActive, &r.DescrOverride,
			&r.PortRole, &r.CLIPortMode); err != nil {
			return nil, err
		}
		m[r.IfIndex] = r
	}
	return m, rows.Err()
}

func (s *Store) UpsertInterfaces(ctx context.Context, deviceID int64, rows []InterfaceUpsert) error {
	if len(rows) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	q := `
		INSERT INTO device_interfaces (
			device_id, if_index, if_descr, if_name, if_type, admin_status, oper_status,
			if_speed, if_high_speed, port_role, hc_in_octets, hc_out_octets, counters_polled_at,
			util_in_pct, util_out_pct, util_max_pct, util_high_active, poe_active, poe_power_w, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,now())
		ON CONFLICT (device_id, if_index) DO UPDATE SET
			if_descr = CASE
				WHEN NULLIF(btrim(device_interfaces.cli_description), '') IS NOT NULL
					AND (
						EXCLUDED.if_descr IS NULL
						OR btrim(COALESCE(EXCLUDED.if_descr, '')) = ''
						OR btrim(COALESCE(EXCLUDED.if_descr, '')) ~* '^Slot:\s*[0-9]+\s+Port:'
						OR lower(btrim(COALESCE(EXCLUDED.if_descr, ''))) LIKE '%gigabit - level%'
					)
				THEN device_interfaces.cli_description
				ELSE EXCLUDED.if_descr
			END,
			if_name = EXCLUDED.if_name,
			if_type = EXCLUDED.if_type,
			admin_status = EXCLUDED.admin_status,
			oper_status = EXCLUDED.oper_status,
			if_speed = EXCLUDED.if_speed,
			if_high_speed = EXCLUDED.if_high_speed,
			port_role = CASE
				WHEN device_interfaces.cli_port_mode IS NOT NULL AND btrim(device_interfaces.cli_port_mode) <> ''
				THEN device_interfaces.port_role
				ELSE EXCLUDED.port_role
			END,
			hc_in_octets = EXCLUDED.hc_in_octets,
			hc_out_octets = EXCLUDED.hc_out_octets,
			counters_polled_at = EXCLUDED.counters_polled_at,
			util_in_pct = EXCLUDED.util_in_pct,
			util_out_pct = EXCLUDED.util_out_pct,
			util_max_pct = EXCLUDED.util_max_pct,
			util_high_active = EXCLUDED.util_high_active,
			poe_active = COALESCE(EXCLUDED.poe_active, device_interfaces.poe_active),
			poe_power_w = COALESCE(EXCLUDED.poe_power_w, device_interfaces.poe_power_w),
			updated_at = now()`
	for _, r := range rows {
		batch.Queue(q, deviceID, r.IfIndex, r.IfDescr, r.IfName, r.IfType, r.AdminStatus, r.OperStatus,
			r.IfSpeed, r.IfHighSpeed, r.PortRole, r.HCInOctets, r.HCOutOctets, r.CountersPolledAt,
			r.UtilInPct, r.UtilOutPct, r.UtilMaxPct, r.UtilHighActive, r.PoeActive, r.PoePowerW)
	}
	br := s.pool.SendBatch(ctx, batch)
	defer br.Close()
	for range rows {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return br.Close()
}

type InterfaceUpsert struct {
	IfIndex            int
	IfDescr            *string
	IfName             *string
	IfType             *int64
	AdminStatus        *int
	OperStatus         *int
	IfSpeed            *int64
	IfHighSpeed        *int64
	PortRole           *string
	HCInOctets         *int64
	HCOutOctets        *int64
	CountersPolledAt   *time.Time
	UtilInPct          *float32
	UtilOutPct         *float32
	UtilMaxPct         *float32
	UtilHighActive     bool
	/** nil — не обновлять поле poe_active (MIB недоступен). */
	PoeActive          *bool
	/** nil — не обновлять поле poe_power_w (метрика недоступна). */
	PoePowerW          *float32
}

func (s *Store) InsertEvent(ctx context.Context, deviceID int64, ifIndex *int, eventType, severity string, payload map[string]interface{}) (int64, error) {
	var jb []byte
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return 0, err
		}
		jb = b
	} else {
		jb = []byte("{}")
	}
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO events (device_id, if_index, event_type, severity, payload)
		VALUES ($1,$2,$3,$4,$5::jsonb)
		RETURNING id`,
		deviceID, ifIndex, eventType, severity, jb,
	).Scan(&id)
	return id, err
}

func (s *Store) HasEventSince(ctx context.Context, deviceID int64, eventType string, since time.Time) (bool, error) {
	if eventType == "" || since.IsZero() {
		return false, nil
	}
	var ok bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM events
			WHERE device_id = $1 AND event_type = $2 AND created_at >= $3
		)`, deviceID, eventType, since,
	).Scan(&ok)
	return ok, err
}

func (s *Store) ListEventsByDevice(ctx context.Context, deviceID int64, limit int, eventType string) ([]models.Event, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	et := strings.TrimSpace(eventType)
	var rows pgx.Rows
	var err error
	if et != "" {
		rows, err = s.pool.Query(ctx, `
			SELECT id, device_id, if_index, event_type, severity, payload, created_at
			FROM events WHERE device_id = $1 AND event_type = $3
			ORDER BY created_at DESC LIMIT $2`, deviceID, limit, et)
	} else {
		rows, err = s.pool.Query(ctx, `
			SELECT id, device_id, if_index, event_type, severity, payload, created_at
			FROM events WHERE device_id = $1
			ORDER BY created_at DESC LIMIT $2`, deviceID, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out, err := scanEvents(rows)
	if err != nil {
		return nil, err
	}
	if err := s.EnrichEventsWithIfaceLabels(ctx, out); err != nil {
		return out, err
	}
	return out, nil
}

func scanEvents(rows pgx.Rows) ([]models.Event, error) {
	var out []models.Event
	for rows.Next() {
		var e models.Event
		var raw []byte
		if err := rows.Scan(&e.ID, &e.DeviceID, &e.IfIndex, &e.EventType, &e.Severity, &raw, &e.CreatedAt); err != nil {
			return nil, err
		}
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &e.Payload)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) ListEvents(ctx context.Context, limit int, deviceID *int64, eventType, severity string) ([]models.Event, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	et := strings.TrimSpace(eventType)
	sev := strings.TrimSpace(severity)
	var dev interface{}
	if deviceID != nil {
		dev = *deviceID
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, device_id, if_index, event_type, severity, payload, created_at
		FROM events
		WHERE ($2::bigint IS NULL OR device_id = $2)
		  AND ($3::text = '' OR event_type = $3)
		  AND ($4::text = '' OR severity = $4)
		ORDER BY created_at DESC LIMIT $1`,
		limit, dev, et, sev)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out, err := scanEvents(rows)
	if err != nil {
		return nil, err
	}
	if err := s.EnrichEventsWithIfaceLabels(ctx, out); err != nil {
		return out, err
	}
	return out, nil
}

// NotificationSettings — глобальные настройки уведомлений (одна строка id=1).
type NotificationSettings struct {
	WebhookURL          *string `json:"webhook_url"`
	WebhookEnabled      bool    `json:"webhook_enabled"`
	WebhookEventTypes   *string `json:"webhook_event_types"`
	WebhookSeverities   *string `json:"webhook_severities"`
	EmailEnabled        bool    `json:"email_enabled"`
	EmailFrom           *string `json:"email_from"`
	EmailTo             *string `json:"email_to"`
	EmailEventTypes     *string `json:"email_event_types"`
	EmailSeverities     *string `json:"email_severities"`
	SMTPHost            *string `json:"smtp_host"`
	SMTPPort            int     `json:"smtp_port"`
	SMTPUsername        *string `json:"smtp_username"`
	SMTPPassword        *string `json:"-"`
	SMTPTLSSkipVerify  bool    `json:"smtp_tls_skip_verify"`
	TelegramBotToken    *string `json:"-"`
	TelegramChatID      *string `json:"telegram_chat_id"`
	TelegramEnabled     bool    `json:"telegram_enabled"`
	TelegramEventTypes  *string `json:"telegram_event_types"`
	TelegramSeverities  *string `json:"telegram_severities"`
	NotifyMaxRetries           int     `json:"notify_max_retries"`
	NotifyRetryBackoffMs       int     `json:"notify_retry_backoff_ms"`
	IncidentActionEnabled          bool    `json:"incident_action_enabled"`
	IncidentActionEventTypes       *string `json:"incident_action_event_types"`
	IncidentActionDryRun           bool    `json:"incident_action_dry_run"`
	IncidentActionCooldownSeconds int     `json:"incident_action_cooldown_seconds"`
}

func (s *Store) GetNotificationSettings(ctx context.Context) (NotificationSettings, error) {
	var ns NotificationSettings
	err := s.pool.QueryRow(ctx, `
		SELECT webhook_url, webhook_enabled,
		       webhook_event_types, webhook_severities,
		       email_enabled, email_from, email_to, email_event_types, email_severities,
		       smtp_host, smtp_port, smtp_username, smtp_password, smtp_tls_skip_verify,
		       telegram_bot_token, telegram_chat_id, telegram_enabled,
		       telegram_event_types, telegram_severities,
		       notify_max_retries, notify_retry_backoff_ms,
		       incident_action_enabled, incident_action_event_types,
		       incident_action_dry_run, incident_action_cooldown_seconds
		FROM notification_settings WHERE id = 1`,
	).Scan(&ns.WebhookURL, &ns.WebhookEnabled,
		&ns.WebhookEventTypes, &ns.WebhookSeverities,
		&ns.EmailEnabled, &ns.EmailFrom, &ns.EmailTo, &ns.EmailEventTypes, &ns.EmailSeverities,
		&ns.SMTPHost, &ns.SMTPPort, &ns.SMTPUsername, &ns.SMTPPassword, &ns.SMTPTLSSkipVerify,
		&ns.TelegramBotToken, &ns.TelegramChatID, &ns.TelegramEnabled,
		&ns.TelegramEventTypes, &ns.TelegramSeverities,
		&ns.NotifyMaxRetries, &ns.NotifyRetryBackoffMs,
		&ns.IncidentActionEnabled, &ns.IncidentActionEventTypes,
		&ns.IncidentActionDryRun, &ns.IncidentActionCooldownSeconds)
	if err == pgx.ErrNoRows {
		return NotificationSettings{}, nil
	}
	return ns, err
}

// UpsertNotificationSettings записывает полную строку настроек (после слияния в обработчике API).
func (s *Store) UpsertNotificationSettings(ctx context.Context, ns NotificationSettings) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO notification_settings (
			id, webhook_url, webhook_enabled,
			webhook_event_types, webhook_severities,
			email_enabled, email_from, email_to, email_event_types, email_severities,
			smtp_host, smtp_port, smtp_username, smtp_password, smtp_tls_skip_verify,
			telegram_bot_token, telegram_chat_id, telegram_enabled,
			telegram_event_types, telegram_severities,
			notify_max_retries, notify_retry_backoff_ms,
			incident_action_enabled, incident_action_event_types,
			incident_action_dry_run, incident_action_cooldown_seconds, updated_at)
		VALUES (1, $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, now())
		ON CONFLICT (id) DO UPDATE SET
			webhook_url = EXCLUDED.webhook_url,
			webhook_enabled = EXCLUDED.webhook_enabled,
			webhook_event_types = EXCLUDED.webhook_event_types,
			webhook_severities = EXCLUDED.webhook_severities,
			email_enabled = EXCLUDED.email_enabled,
			email_from = EXCLUDED.email_from,
			email_to = EXCLUDED.email_to,
			email_event_types = EXCLUDED.email_event_types,
			email_severities = EXCLUDED.email_severities,
			smtp_host = EXCLUDED.smtp_host,
			smtp_port = EXCLUDED.smtp_port,
			smtp_username = EXCLUDED.smtp_username,
			smtp_password = EXCLUDED.smtp_password,
			smtp_tls_skip_verify = EXCLUDED.smtp_tls_skip_verify,
			telegram_bot_token = EXCLUDED.telegram_bot_token,
			telegram_chat_id = EXCLUDED.telegram_chat_id,
			telegram_enabled = EXCLUDED.telegram_enabled,
			telegram_event_types = EXCLUDED.telegram_event_types,
			telegram_severities = EXCLUDED.telegram_severities,
			notify_max_retries = EXCLUDED.notify_max_retries,
			notify_retry_backoff_ms = EXCLUDED.notify_retry_backoff_ms,
			incident_action_enabled = EXCLUDED.incident_action_enabled,
			incident_action_event_types = EXCLUDED.incident_action_event_types,
			incident_action_dry_run = EXCLUDED.incident_action_dry_run,
			incident_action_cooldown_seconds = EXCLUDED.incident_action_cooldown_seconds,
			updated_at = now()`,
		ns.WebhookURL, ns.WebhookEnabled,
		ns.WebhookEventTypes, ns.WebhookSeverities,
		ns.EmailEnabled, ns.EmailFrom, ns.EmailTo, ns.EmailEventTypes, ns.EmailSeverities,
		ns.SMTPHost, ns.SMTPPort, ns.SMTPUsername, ns.SMTPPassword, ns.SMTPTLSSkipVerify,
		ns.TelegramBotToken, ns.TelegramChatID, ns.TelegramEnabled,
		ns.TelegramEventTypes, ns.TelegramSeverities,
		ns.NotifyMaxRetries, ns.NotifyRetryBackoffMs,
		ns.IncidentActionEnabled, ns.IncidentActionEventTypes,
		ns.IncidentActionDryRun, ns.IncidentActionCooldownSeconds,
	)
	return err
}

// UISPSettingsRow — строка uisp_settings (id=1).
type UISPSettingsRow struct {
	Enabled         bool
	BaseURL         *string
	APIToken        *string
	ImportCommunity string
}

func (s *Store) GetUISPSettings(ctx context.Context) (UISPSettingsRow, error) {
	var r UISPSettingsRow
	err := s.pool.QueryRow(ctx, `
		SELECT enabled, base_url, api_token, import_community
		FROM uisp_settings WHERE id = 1`,
	).Scan(&r.Enabled, &r.BaseURL, &r.APIToken, &r.ImportCommunity)
	if err == pgx.ErrNoRows {
		return UISPSettingsRow{ImportCommunity: "public"}, nil
	}
	return r, err
}

func (s *Store) UpsertUISPSettings(ctx context.Context, enabled bool, baseURL, apiToken *string, importCommunity string) error {
	if strings.TrimSpace(importCommunity) == "" {
		importCommunity = "public"
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO uisp_settings (id, enabled, base_url, api_token, import_community, updated_at)
		VALUES (1, $1, $2, $3, $4, now())
		ON CONFLICT (id) DO UPDATE SET
			enabled = EXCLUDED.enabled,
			base_url = EXCLUDED.base_url,
			api_token = EXCLUDED.api_token,
			import_community = EXCLUDED.import_community,
			updated_at = now()`,
		enabled, baseURL, apiToken, importCommunity,
	)
	return err
}

// TopologySettings — глобальные UI-настройки карты топологии (id=1).
type TopologySettings struct {
	RootDeviceID *int64 `json:"root_device_id"`
}

func (s *Store) GetTopologySettings(ctx context.Context) (TopologySettings, error) {
	var out TopologySettings
	err := s.pool.QueryRow(ctx, `
		SELECT root_device_id FROM topology_settings WHERE id = 1`,
	).Scan(&out.RootDeviceID)
	if err == pgx.ErrNoRows {
		return TopologySettings{}, nil
	}
	return out, err
}

func (s *Store) SetTopologyRootDeviceID(ctx context.Context, rootDeviceID *int64) error {
	if rootDeviceID != nil && *rootDeviceID > 0 {
		var exists bool
		if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM devices WHERE id = $1)`, *rootDeviceID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("устройство #%d не найдено", *rootDeviceID)
		}
	} else {
		rootDeviceID = nil
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO topology_settings (id, root_device_id, updated_at)
		VALUES (1, $1, now())
		ON CONFLICT (id) DO UPDATE SET
			root_device_id = EXCLUDED.root_device_id,
			updated_at = now()`,
		rootDeviceID,
	)
	return err
}

// UpsertSwitchFromUISP создаёт или обновляет узел по uisp_device_id (SNMP v2c + community из настроек UISP).
func (s *Store) UpsertSwitchFromUISP(ctx context.Context, name, host, location, community, uispDeviceID string) (created bool, err error) {
	comm := strings.TrimSpace(community)
	if comm == "" {
		comm = "public"
	}
	host = strings.TrimSpace(host)
	name = strings.TrimSpace(name)
	uispDeviceID = strings.TrimSpace(uispDeviceID)
	var loc interface{}
	if strings.TrimSpace(location) == "" {
		loc = nil
	} else {
		loc = strings.TrimSpace(location)
	}

	var existingID int64
	err = s.pool.QueryRow(ctx, `SELECT id FROM devices WHERE uisp_device_id = $1`, uispDeviceID).Scan(&existingID)
	if err == nil {
		if err := s.CheckDeviceIdentity(ctx, host, nil, existingID); err != nil {
			// host занят другим узлом — обновляем имя/location, host не трогаем
			_, err2 := s.pool.Exec(ctx, `
				UPDATE devices SET
					name = $1,
					location = $2,
					community = CASE WHEN snmp_version IN ('v1', 'v2c') THEN $3 ELSE community END,
					uisp_overview_status = 'active',
					updated_at = now()
				WHERE id = $4`,
				name, loc, comm, existingID,
			)
			return false, err2
		}
		_, err = s.pool.Exec(ctx, `
			UPDATE devices SET
				name = $1,
				host = $2,
				location = $3,
				community = CASE WHEN snmp_version IN ('v1', 'v2c') THEN $4 ELSE community END,
				uisp_overview_status = 'active',
				updated_at = now()
			WHERE id = $5`,
			name, host, loc, comm, existingID,
		)
		return false, mapUniqueViolation(err)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	if err := s.CheckDeviceIdentity(ctx, host, nil, 0); err != nil {
		// не создаём второй узел на тот же IP
		return false, nil
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO devices (name, host, location, uisp_device_id, uisp_overview_status, snmp_version, community, poll_interval_seconds)
		VALUES ($1, $2, $3, $4, 'active', 'v2c', $5, 60)`,
		name, host, loc, uispDeviceID, comm,
	)
	if err != nil {
		return false, mapUniqueViolation(err)
	}
	return true, nil
}

// ApplyUISPOverviewStatuses обновляет uisp_overview_status по UUID из UISP (только строки с совпадающим uisp_device_id).
func (s *Store) ApplyUISPOverviewStatuses(ctx context.Context, byUUID map[string]string) (int64, error) {
	var n int64
	for uuid, status := range byUUID {
		uuid = strings.TrimSpace(uuid)
		if uuid == "" {
			continue
		}
		st := strings.TrimSpace(status)
		var val interface{}
		if st == "" {
			val = nil
		} else {
			val = st
		}
		tag, err := s.pool.Exec(ctx, `
			UPDATE devices SET uisp_overview_status = $2, updated_at = now()
			WHERE uisp_device_id = $1`,
			uuid, val,
		)
		if err != nil {
			return n, err
		}
		n += tag.RowsAffected()
	}
	return n, nil
}

// UpdateDeviceChassisMAC задает или очищает chassis_mac (пустая строка → NULL).
func (s *Store) UpdateDeviceChassisMAC(ctx context.Context, id int64, raw string) error {
	raw = strings.TrimSpace(raw)
	var macVal interface{}
	if raw != "" {
		mac, ok := NormalizeMACQuery(raw)
		if !ok {
			return errors.New("chassis_mac: неверный MAC")
		}
		if err := s.CheckDeviceIdentity(ctx, "", &mac, id); err != nil {
			return err
		}
		macVal = mac
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE devices SET
			chassis_mac = $2,
			updated_at = now()
		WHERE id = $1`,
		id, macVal,
	)
	if err != nil {
		return mapUniqueViolation(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrDeviceNotFound
	}
	return nil
}

// UpdateDeviceHost обновляет IP/host (пустая строка → NULL, узел без адреса).
func (s *Store) UpdateDeviceHost(ctx context.Context, id int64, host string) error {
	host = strings.TrimSpace(host)
	if host != "" {
		if err := s.CheckDeviceIdentity(ctx, host, nil, id); err != nil {
			return err
		}
	}
	// Пустой host → NULL через NULLIF; ::text нужен, иначе PG не выводит тип параметра (42P08).
	tag, err := s.pool.Exec(ctx, `
		UPDATE devices SET
			host = NULLIF(btrim($2::text), ''),
			last_ping_ok = CASE WHEN NULLIF(btrim($2::text), '') IS NULL THEN NULL ELSE last_ping_ok END,
			last_ping_at = CASE WHEN NULLIF(btrim($2::text), '') IS NULL THEN NULL ELSE last_ping_at END,
			last_ping_rtt_ms = CASE WHEN NULLIF(btrim($2::text), '') IS NULL THEN NULL ELSE last_ping_rtt_ms END,
			updated_at = now()
		WHERE id = $1`,
		id, host,
	)
	if err != nil {
		return mapUniqueViolation(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrDeviceNotFound
	}
	return nil
}

// UpdateDeviceOnlineOverride: mode auto|online|offline (авто / вручную онлайн / вручную оффлайн).
func (s *Store) UpdateDeviceOnlineOverride(ctx context.Context, id int64, mode string) error {
	mode = strings.ToLower(strings.TrimSpace(mode))
	var val interface{}
	switch mode {
	case "auto", "":
		val = nil
	case "online":
		t := true
		val = t
	case "offline":
		f := false
		val = f
	default:
		return errors.New("mode: ожидается auto, online или offline")
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE devices SET online_override = $2, updated_at = now()
		WHERE id = $1`, id, val)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrDeviceNotFound
	}
	return s.RefreshOfflineSince(ctx, id)
}

// UpdateDeviceTrustLinkTraps — флаг мгновенных LINK_* из trap при mode=per_device.
func (s *Store) UpdateDeviceTrustLinkTraps(ctx context.Context, id int64, trust bool) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE devices SET trust_link_traps = $2, updated_at = now()
		WHERE id = $1`, id, trust)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrDeviceNotFound
	}
	return nil
}

// UpdateDeviceName обновляет только отображаемое имя узла (обязательное, непустое).
func (s *Store) UpdateDeviceName(ctx context.Context, id int64, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("name: пустое имя недопустимо")
	}
	tag, err := s.pool.Exec(ctx, `UPDATE devices SET name = $2, updated_at = now() WHERE id = $1`, id, name)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrDeviceNotFound
	}
	return nil
}

// UpdateDeviceLocation обновляет только поле location (пустая строка → NULL).
func (s *Store) UpdateDeviceLocation(ctx context.Context, id int64, location *string) error {
	var loc interface{}
	if location == nil {
		loc = nil
	} else if strings.TrimSpace(*location) == "" {
		loc = nil
	} else {
		t := strings.TrimSpace(*location)
		loc = t
	}
	tag, err := s.pool.Exec(ctx, `UPDATE devices SET location = $2, updated_at = now() WHERE id = $1`, id, loc)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrDeviceNotFound
	}
	return nil
}

func (s *Store) UpdateDeviceCategory(ctx context.Context, id int64, category string) error {
	cat := NormalizeDeviceCategory(category)
	tag, err := s.pool.Exec(ctx, `UPDATE devices SET device_category = $2, updated_at = now() WHERE id = $1`, id, cat)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrDeviceNotFound
	}
	return nil
}

// UpdateDevicePollInterval обновляет только poll_interval_seconds (ограничение БД: 10–86400).
func (s *Store) UpdateDevicePollInterval(ctx context.Context, id int64, seconds int) error {
	if seconds < 10 || seconds > 86400 {
		return errors.New("poll_interval_seconds: ожидается 10–86400")
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE devices SET poll_interval_seconds = $2, updated_at = now() WHERE id = $1`, id, seconds)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrDeviceNotFound
	}
	return nil
}

func (s *Store) ListInterfacesByDevice(ctx context.Context, deviceID int64) ([]models.DeviceInterface, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT di.device_id, di.if_index, di.if_descr, di.if_name, di.if_type, di.admin_status, di.oper_status,
		       di.if_speed, di.if_high_speed, di.port_role, di.hc_in_octets, di.hc_out_octets, di.counters_polled_at,
		       di.util_in_pct, di.util_out_pct, di.util_max_pct, di.util_high_active,
		       di.util_high_pct AS threshold_high_pct, di.util_ok_pct AS threshold_ok_pct,
		       di.poe_active, di.poe_power_w, di.updated_at,
		       (pei.if_index IS NOT NULL) AS event_ignored,
		       di.descr_override, di.cli_description,
		       di.cli_port_mode, di.cli_access_vlan, di.cli_mode_synced_at
		FROM device_interfaces di
		LEFT JOIN port_event_ignore pei ON pei.device_id = di.device_id AND pei.if_index = di.if_index
		WHERE di.device_id = $1 ORDER BY di.if_index`, deviceID)
	if err != nil {
		return nil, err
	}
	var out []models.DeviceInterface
	for rows.Next() {
		var r models.DeviceInterface
		if err := rows.Scan(
			&r.DeviceID, &r.IfIndex, &r.IfDescr, &r.IfName, &r.IfType, &r.AdminStatus, &r.OperStatus,
			&r.IfSpeed, &r.IfHighSpeed, &r.PortRole, &r.HCInOctets, &r.HCOutOctets, &r.CountersPolledAt,
			&r.UtilInPct, &r.UtilOutPct, &r.UtilMaxPct, &r.UtilHighActive,
			&r.UtilHighPct, &r.UtilOkPct,
			&r.PoeActive, &r.PoePowerW, &r.UpdatedAt, &r.EventIgnored, &r.DescrOverride, &r.CLIDescription,
			&r.CLIPortMode, &r.CliAccessVlan, &r.CliModeSyncedAt,
		); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	// Важно закрыть rows до второго Query — иначе при малом пуле PG возможен deadlock
	// (все коннекты заняты открытыми курсорами, VLAN-запрос ждёт свободный).
	// VLAN — доп. поле; сбой не должен ломать список портов.
	vlans, err := s.DominantVLANsForDevice(ctx, deviceID)
	if err != nil {
		return out, nil
	}
	for i := range out {
		role := strings.ToLower(strings.TrimSpace(out[i].PortRole))
		if role == "access" && out[i].CliAccessVlan != nil && *out[i].CliAccessVlan > 0 {
			vv := *out[i].CliAccessVlan
			out[i].VlanID = &vv
			continue
		}
		if v, ok := vlans[out[i].IfIndex]; ok {
			vv := v
			out[i].VlanID = &vv
		}
	}
	return out, nil
}

// DominantVLANsForDevice: if_index → vlan_id с наибольшим числом FDB-записей.
func (s *Store) DominantVLANsForDevice(ctx context.Context, deviceID int64) (map[int]int, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT ON (if_index) if_index, vlan_id
		FROM (
			SELECT if_index, vlan_id, COUNT(*) AS c
			FROM device_fdb_entries
			WHERE device_id = $1 AND vlan_id IS NOT NULL
			GROUP BY if_index, vlan_id
		) t
		ORDER BY if_index, c DESC`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int]int)
	for rows.Next() {
		var ifIndex, vlan int
		if err := rows.Scan(&ifIndex, &vlan); err != nil {
			return out, err
		}
		out[ifIndex] = vlan
	}
	return out, rows.Err()
}

const descrOverrideMaxLen = 200

// NormalizeDescrOverride пустая строка → nil (снова SNMP); иначе обрезка пробелов и длины.
func NormalizeDescrOverride(raw string) *string {
	t := strings.TrimSpace(raw)
	if t == "" {
		return nil
	}
	runes := []rune(t)
	if len(runes) > descrOverrideMaxLen {
		t = string(runes[:descrOverrideMaxLen])
	}
	return &t
}

func (s *Store) UpdateInterfaceDescrOverride(ctx context.Context, deviceID int64, ifIndex int, descr *string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE device_interfaces SET descr_override = $3, updated_at = now()
		WHERE device_id = $1 AND if_index = $2`, deviceID, ifIndex, descr)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrDeviceNotFound
	}
	return nil
}

// SyncInterfaceDescrAfterDeviceWrite: описание уже на свитче — if_descr + cli_description, снимаем override.
func (s *Store) SyncInterfaceDescrAfterDeviceWrite(ctx context.Context, deviceID int64, ifIndex int, descr string) error {
	d := strings.TrimSpace(descr)
	var descrArg interface{}
	if d == "" {
		descrArg = nil
	} else {
		descrArg = d
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE device_interfaces
		SET if_descr = $3, cli_description = $3, descr_override = NULL, updated_at = now()
		WHERE device_id = $1 AND if_index = $2`, deviceID, ifIndex, descrArg)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrDeviceNotFound
	}
	return nil
}

func (s *Store) UpdateInterfaceAdminStatus(ctx context.Context, deviceID int64, ifIndex, adminStatus int) error {
	if adminStatus != 1 && adminStatus != 2 {
		return fmt.Errorf("admin_status: 1 или 2")
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE device_interfaces SET admin_status = $3, updated_at = now()
		WHERE device_id = $1 AND if_index = $2`, deviceID, ifIndex, adminStatus)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrDeviceNotFound
	}
	return nil
}

func (s *Store) GetInterfaceName(ctx context.Context, deviceID int64, ifIndex int) (string, error) {
	var name, descr *string
	err := s.pool.QueryRow(ctx, `
		SELECT if_name, if_descr FROM device_interfaces WHERE device_id = $1 AND if_index = $2`,
		deviceID, ifIndex).Scan(&name, &descr)
	if err != nil {
		return "", err
	}
	if name != nil && strings.TrimSpace(*name) != "" {
		return strings.TrimSpace(*name), nil
	}
	// EdgeSwitch / часть MIB: ifName пуст, ifDescr = "Port 1" → CLI "0/1".
	if descr != nil {
		d := strings.TrimSpace(*descr)
		if len(d) >= 6 && strings.EqualFold(d[:5], "port ") {
			n := strings.TrimSpace(d[5:])
			if n != "" && onlyDigits(n) {
				return "0/" + n, nil
			}
		}
	}
	return "", fmt.Errorf("у порта нет if_name")
}

func onlyDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}

func (s *Store) UpdateDeviceMonitoring(ctx context.Context, id int64, utilHigh, utilOk *float32, fdbPollSec *int) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE devices SET
			util_high_pct = $2,
			util_ok_pct = $3,
			fdb_poll_interval_seconds = $4,
			updated_at = now()
		WHERE id = $1`, id, utilHigh, utilOk, fdbPollSec)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrDeviceNotFound
	}
	return nil
}

func (s *Store) UpdateInterfaceThresholds(ctx context.Context, deviceID int64, ifIndex int, utilHigh, utilOk *float32) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE device_interfaces SET util_high_pct = $3, util_ok_pct = $4, updated_at = now()
		WHERE device_id = $1 AND if_index = $2`, deviceID, ifIndex, utilHigh, utilOk)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrDeviceNotFound
	}
	return nil
}

// ErrDeviceNotFound — нет строки devices с таким id.
var ErrDeviceNotFound = errors.New("device not found")

var ErrAuthUserNotFound = errors.New("auth user not found")
var ErrAuthSessionNotFound = errors.New("auth session not found")
var ErrLastAdmin = errors.New("нельзя удалить или понизить последнего активного администратора")

func (s *Store) DeleteDevice(ctx context.Context, id int64) error {
	// Сначала вернём кандидатов «Обнаружено» в new — иначе после ON DELETE SET NULL
	// статус останется added без узла.
	if _, err := s.pool.Exec(ctx, `
		UPDATE discovered_devices SET
			status = $2,
			promoted_device_id = NULL,
			updated_at = now()
		WHERE promoted_device_id = $1`, id, DiscoveredStatusNew); err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM devices WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrDeviceNotFound
	}
	return nil
}

// DeleteAllDevices удаляет все узлы (события и порты каскадом по FK).
func (s *Store) DeleteAllDevices(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM devices`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

