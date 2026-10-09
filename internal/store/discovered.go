package store

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/PTah/netlynx/internal/models"
	"github.com/jackc/pgx/v5"
)

const (
	DiscoveredStatusNew     = "new"
	DiscoveredStatusIgnored = "ignored"
	DiscoveredStatusAdded   = "added"
)

type DiscoveredDevice struct {
	ID                    int64     `json:"id"`
	IdentityKey           string    `json:"identity_key"`
	RemoteSysName         *string   `json:"remote_sys_name,omitempty"`
	RemoteChassisID       *string   `json:"remote_chassis_id,omitempty"`
	RemoteMgmtAddr        *string   `json:"remote_mgmt_addr,omitempty"`
	FirstSeenFromDeviceID *int64    `json:"first_seen_from_device_id,omitempty"`
	FirstSeenIfIndex      *int      `json:"first_seen_if_index,omitempty"`
	LastSeenFromDeviceID  *int64    `json:"last_seen_from_device_id,omitempty"`
	LastSeenIfIndex       *int      `json:"last_seen_if_index,omitempty"`
	LastProtocol          *string   `json:"last_protocol,omitempty"`
	Status                string    `json:"status"`
	PromotedDeviceID      *int64    `json:"promoted_device_id,omitempty"`
	LastSeenAt            time.Time `json:"last_seen_at"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
	// Joined for UI
	SeenFromName *string `json:"seen_from_name,omitempty"`
	// FDB на том же свитче/порту указывает ровно на один узел inventory (слабый LLDP без MAC).
	LikelyDeviceID   *int64  `json:"likely_device_id,omitempty"`
	LikelyDeviceName *string `json:"likely_device_name,omitempty"`
}

// DiscoveredIdentityKey выбирает стабильный ключ дедупа.
// Порядок: полный MAC (chassis) → mgmt IP → sysName → короткий chassis.
// Иначе заводские имена вроде SIP-T41S / X210-V2 схлопывают десятки телефонов в один узел.
func DiscoveredIdentityKey(sysName, mgmtAddr, chassisID string) string {
	return DiscoveredIdentityKeyWithPort(sysName, mgmtAddr, chassisID, "")
}

// DiscoveredIdentityKeyWithPort — как DiscoveredIdentityKey, плюс Port ID как MAC
// (Yealink и др. часто кладут MAC телефона в lldpRemPortId, а не в chassis).
func DiscoveredIdentityKeyWithPort(sysName, mgmtAddr, chassisID, portID string) string {
	ch := strings.TrimSpace(chassisID)
	if ch == "" {
		if mac, ok := NormalizeMACQuery(portID); ok {
			if h := macHexDigits(mac); len(h) == 12 {
				ch = mac
			}
		}
	}
	// Полный MAC — hex без разделителей, чтобы 345a… и 34:5a:… давали один ключ.
	if h := macHexDigits(ch); len(h) == 12 {
		return "chassis:" + h
	}
	// LLDP networkAddress (01+IPv4), ошибочно лежавший в chassis.
	if ip := decodeDiscoveredNetworkHex(ch); ip != "" {
		return "addr:" + strings.ToLower(ip)
	}
	mgmt := strings.ToLower(strings.TrimSpace(mgmtAddr))
	if mgmt != "" {
		return "addr:" + mgmt
	}
	if k := normalizeDeviceKey(sysName); k != "" {
		return "name:" + k
	}
	if h := macHexDigits(ch); len(h) >= 6 && len(h) <= 12 && len(h)%2 == 0 {
		return "chassis:" + h
	}
	ch = strings.ToLower(ch)
	if ch == "" {
		return ""
	}
	return "chassis:" + ch
}

func normalizeDiscoveredChassis(ch *string) *string {
	if ch == nil {
		return nil
	}
	raw := strings.TrimSpace(*ch)
	if raw == "" {
		return nil
	}
	// LLDP networkAddress, ошибочно сохранённый как «MAC» 01:c0:a8:… — не chassis.
	if ip := decodeDiscoveredNetworkHex(raw); ip != "" {
		return nil
	}
	// Полный и укороченный MAC (6–12 hex, чётная длина) → aa:bb:…
	if mac, ok := NormalizeMACQuery(raw); ok {
		// 10 hex с префиксом 01 — уже отсеяли выше; остальное ок
		h := macHexDigits(mac)
		if len(h) == 10 && strings.HasPrefix(h, "01") {
			return nil
		}
		return &mac
	}
	return &raw
}

// decodeDiscoveredNetworkHex — копия эвристики snmp (store не импортирует snmp).
func decodeDiscoveredNetworkHex(raw string) string {
	hex := macHexDigits(raw)
	if len(hex) != 10 || !strings.HasPrefix(hex, "01") {
		return ""
	}
	var b [4]byte
	for i := 0; i < 4; i++ {
		v, err := strconv.ParseUint(hex[2+i*2:4+i*2], 16, 8)
		if err != nil {
			return ""
		}
		b[i] = byte(v)
	}
	ip := net.IP(b[:]).String()
	if net.ParseIP(ip) == nil {
		return ""
	}
	return ip
}

// ShouldOfferDiscovered — кандидат, если не резолвится в известный узел и есть identity.
func ShouldOfferDiscovered(idx deviceNameIndex, nb PortNeighbor) (identity string, ok bool) {
	identity = DiscoveredIdentityKeyWithPort(
		derefStr(nb.RemoteSysName),
		derefStr(nb.RemoteMgmtAddr),
		derefStr(nb.RemoteChassisID),
		derefStr(nb.RemotePortID),
	)
	if identity == "" {
		return "", false
	}
	if _, known := resolveRemoteDeviceID(idx, nb); known {
		return "", false
	}
	return identity, true
}

// SyncDiscoveredFromNeighbors обновляет кандидатов по снимку соседей источника.
func (s *Store) SyncDiscoveredFromNeighbors(ctx context.Context, sourceDeviceID int64, neighbors []PortNeighbor) error {
	devices, err := s.ListDevices(ctx)
	if err != nil {
		return err
	}
	idx := buildDeviceNameIndex(devices)
	ignored, err := s.loadIgnoredDiscoveredKeySet(ctx)
	if err != nil {
		return err
	}
	now := time.Now().UTC()

	for _, nb := range neighbors {
		nb = s.enrichNeighborFromPortFDB(ctx, sourceDeviceID, nb)
		identity, offer := ShouldOfferDiscovered(idx, nb)
		if !offer {
			continue
		}
		// Уже в ignore (тот же или смежный identity) — не создавать снова как new.
		if discoveredKeySetHits(ignored, neighborDiscoveredIgnoreKeys(nb)) {
			continue
		}
		if err := s.upsertDiscoveredCandidate(ctx, identity, sourceDeviceID, nb, now); err != nil {
			return err
		}
	}
	_, err = s.markDiscoveredAlreadyInInventory(ctx, devices)
	return err
}

// enrichNeighborFromPortFDB — LLDP без MAC/mgmt: подставить chassis/host единственного inventory на порту.
func (s *Store) enrichNeighborFromPortFDB(ctx context.Context, sourceDeviceID int64, nb PortNeighbor) PortNeighbor {
	if !neighborLacksStrongIdentity(nb) || nb.IfIndex <= 0 || sourceDeviceID <= 0 {
		return nb
	}
	hint, err := s.UniqueInventoryOnPort(ctx, sourceDeviceID, nb.IfIndex)
	if err != nil || hint == nil {
		return nb
	}
	if hint.ChassisMAC != "" && strings.TrimSpace(derefStr(nb.RemoteChassisID)) == "" {
		m := hint.ChassisMAC
		nb.RemoteChassisID = &m
	}
	if hint.Host != "" && strings.TrimSpace(derefStr(nb.RemoteMgmtAddr)) == "" {
		h := hint.Host
		nb.RemoteMgmtAddr = &h
	}
	return nb
}

func neighborLacksStrongIdentity(nb PortNeighbor) bool {
	if mac, ok := NormalizeMACQuery(derefStr(nb.RemoteChassisID)); ok && len(macHexDigits(mac)) == 12 {
		return false
	}
	if mac, ok := NormalizeMACQuery(derefStr(nb.RemotePortID)); ok && len(macHexDigits(mac)) == 12 {
		return false
	}
	if strings.TrimSpace(derefStr(nb.RemoteMgmtAddr)) != "" {
		return false
	}
	if ip := decodeDiscoveredNetworkHex(derefStr(nb.RemoteChassisID)); ip != "" {
		return false
	}
	return true
}

func discoveredLacksStrongIdentity(d DiscoveredDevice) bool {
	if mac := DiscoveredChassisMAC(&d); mac != "" && len(macHexDigits(mac)) == 12 {
		return false
	}
	if strings.TrimSpace(derefStr(d.RemoteMgmtAddr)) != "" {
		return false
	}
	key := strings.ToLower(strings.TrimSpace(d.IdentityKey))
	if strings.HasPrefix(key, "addr:") {
		return false
	}
	if strings.HasPrefix(key, "chassis:") {
		if mac, ok := NormalizeMACQuery(strings.TrimPrefix(key, "chassis:")); ok && len(macHexDigits(mac)) == 12 {
			return false
		}
	}
	return true
}

func discoveredPortAnchor(d DiscoveredDevice) (switchID int64, ifIndex int, ok bool) {
	if d.LastSeenFromDeviceID != nil && *d.LastSeenFromDeviceID > 0 && d.LastSeenIfIndex != nil && *d.LastSeenIfIndex > 0 {
		return *d.LastSeenFromDeviceID, *d.LastSeenIfIndex, true
	}
	if d.FirstSeenFromDeviceID != nil && *d.FirstSeenFromDeviceID > 0 && d.FirstSeenIfIndex != nil && *d.FirstSeenIfIndex > 0 {
		return *d.FirstSeenFromDeviceID, *d.FirstSeenIfIndex, true
	}
	return 0, 0, false
}

// loadIgnoredDiscoveredKeySet — ключи ignored-кандидатов (identity / chassis / addr),
// чтобы не показывать их в топологии и не заводить дубликаты new.
func (s *Store) loadIgnoredDiscoveredKeySet(ctx context.Context) (map[string]struct{}, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT identity_key, remote_chassis_id, remote_mgmt_addr, remote_sys_name
		FROM discovered_devices WHERE status = $1`, DiscoveredStatusIgnored)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]struct{}{}
	for rows.Next() {
		var identity string
		var chassis, mgmt, sys *string
		if err := rows.Scan(&identity, &chassis, &mgmt, &sys); err != nil {
			return nil, err
		}
		addDiscoveredIgnoreKeys(out, identity, derefStr(chassis), derefStr(mgmt), derefStr(sys))
	}
	return out, rows.Err()
}

// addDiscoveredIgnoreKeys индексирует стабильные идентификаторы кандидата для match ignore.
// Model-only sysName (SIP-T41S) не добавляем отдельно — только если identity уже name:…
func addDiscoveredIgnoreKeys(out map[string]struct{}, identity, chassis, mgmt, sysName string) {
	add := func(k string) {
		k = strings.TrimSpace(strings.ToLower(k))
		if k == "" {
			return
		}
		out[k] = struct{}{}
	}
	idKey := strings.TrimSpace(strings.ToLower(identity))
	if idKey != "" {
		add(idKey)
	}
	if mac, ok := NormalizeMACQuery(chassis); ok {
		if h := macHexDigits(mac); len(h) == 12 {
			add("chassis:" + h)
		}
	}
	if strings.HasPrefix(idKey, "chassis:") {
		if mac, ok := NormalizeMACQuery(strings.TrimPrefix(idKey, "chassis:")); ok {
			if h := macHexDigits(mac); len(h) == 12 {
				add("chassis:" + h)
			}
		}
	}
	mgmt = strings.ToLower(strings.TrimSpace(mgmt))
	if mgmt != "" {
		add("addr:" + mgmt)
	}
	if strings.HasPrefix(idKey, "addr:") {
		add(idKey)
	}
	_ = sysName // только через identity name: — см. neighborDiscoveredIgnoreKeys
}

// neighborDiscoveredIgnoreKeys — набор ключей соседа для проверки «уже ignored».
func neighborDiscoveredIgnoreKeys(nb PortNeighbor) []string {
	sys := derefStr(nb.RemoteSysName)
	mgmt := derefStr(nb.RemoteMgmtAddr)
	ch := derefStr(nb.RemoteChassisID)
	port := derefStr(nb.RemotePortID)
	identity := DiscoveredIdentityKeyWithPort(sys, mgmt, ch, port)
	tmp := map[string]struct{}{}
	addDiscoveredIgnoreKeys(tmp, identity, ch, mgmt, sys)
	if mac, ok := NormalizeMACQuery(port); ok {
		if h := macHexDigits(mac); len(h) == 12 {
			tmp["chassis:"+h] = struct{}{}
		}
	}
	if ip := decodeDiscoveredNetworkHex(ch); ip != "" {
		tmp["addr:"+strings.ToLower(ip)] = struct{}{}
	}
	out := make([]string, 0, len(tmp))
	for k := range tmp {
		out = append(out, k)
	}
	return out
}

func discoveredKeySetHits(ignored map[string]struct{}, keys []string) bool {
	if len(ignored) == 0 || len(keys) == 0 {
		return false
	}
	for _, k := range keys {
		k = strings.TrimSpace(strings.ToLower(k))
		if k == "" {
			continue
		}
		if _, ok := ignored[k]; ok {
			return true
		}
	}
	return false
}

// IgnoreDiscovered помечает кандидата ignored и все new с тем же chassis/mgmt.
func (s *Store) IgnoreDiscovered(ctx context.Context, id int64) error {
	d, err := s.GetDiscovered(ctx, id)
	if err != nil {
		return err
	}
	if d == nil {
		return ErrDeviceNotFound
	}
	if err := s.SetDiscoveredStatus(ctx, id, DiscoveredStatusIgnored, nil); err != nil {
		return err
	}
	keys := map[string]struct{}{}
	addDiscoveredIgnoreKeys(keys, d.IdentityKey, derefStr(d.RemoteChassisID), derefStr(d.RemoteMgmtAddr), derefStr(d.RemoteSysName))
	if len(keys) == 0 {
		return nil
	}
	list, err := s.ListDiscovered(ctx, DiscoveredStatusNew)
	if err != nil {
		return err
	}
	for _, o := range list {
		if o.ID == id {
			continue
		}
		probe := map[string]struct{}{}
		addDiscoveredIgnoreKeys(probe, o.IdentityKey, derefStr(o.RemoteChassisID), derefStr(o.RemoteMgmtAddr), derefStr(o.RemoteSysName))
		hit := false
		for k := range probe {
			if _, ok := keys[k]; ok {
				hit = true
				break
			}
		}
		if !hit {
			continue
		}
		if err := s.SetDiscoveredStatus(ctx, o.ID, DiscoveredStatusIgnored, nil); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) upsertDiscoveredCandidate(ctx context.Context, identity string, sourceDeviceID int64, nb PortNeighbor, at time.Time) error {
	chassis := normalizeDiscoveredChassis(nb.RemoteChassisID)
	mgmt := nb.RemoteMgmtAddr
	if derefStr(mgmt) == "" {
		if ip := decodeDiscoveredNetworkHex(derefStr(nb.RemoteChassisID)); ip != "" {
			mgmt = &ip
		}
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO discovered_devices (
			identity_key, remote_sys_name, remote_chassis_id, remote_mgmt_addr,
			first_seen_from_device_id, first_seen_if_index,
			last_seen_from_device_id, last_seen_if_index, last_protocol,
			status, last_seen_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$5,$6,$7,'new',$8,$8,$8)
		ON CONFLICT (identity_key) DO UPDATE SET
			remote_sys_name = COALESCE(EXCLUDED.remote_sys_name, discovered_devices.remote_sys_name),
			remote_chassis_id = EXCLUDED.remote_chassis_id,
			remote_mgmt_addr = COALESCE(EXCLUDED.remote_mgmt_addr, discovered_devices.remote_mgmt_addr),
			last_seen_from_device_id = EXCLUDED.last_seen_from_device_id,
			last_seen_if_index = EXCLUDED.last_seen_if_index,
			last_protocol = EXCLUDED.last_protocol,
			last_seen_at = EXCLUDED.last_seen_at,
			updated_at = EXCLUDED.updated_at
			-- status / promoted_device_id не трогаем (ignored/added сохраняются)
		`,
		identity, nb.RemoteSysName, chassis, mgmt,
		sourceDeviceID, nb.IfIndex, nullIfEmpty(nb.Protocol), at,
	)
	return err
}

const DiscoveredProtocolSNMPScan = "snmp-scan"

// UpsertDiscoveredFromScan пишет кандидата с identity addr:<ip> после успешной SNMP-пробы.
// source device/if_index пустые (скан не с порта свитча). status/promoted не затираются.
// Если IP/identity уже ignored — не создаём new, возвращаем существующий id.
func (s *Store) UpsertDiscoveredFromScan(ctx context.Context, host, sysName string) (id int64, err error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return 0, fmt.Errorf("пустой host")
	}
	identity := DiscoveredIdentityKey("", host, "")
	if identity == "" {
		return 0, fmt.Errorf("не удалось построить identity для %s", host)
	}
	ignored, err := s.loadIgnoredDiscoveredKeySet(ctx)
	if err != nil {
		return 0, err
	}
	probe := map[string]struct{}{}
	addDiscoveredIgnoreKeys(probe, identity, "", host, sysName)
	keys := make([]string, 0, len(probe))
	for k := range probe {
		keys = append(keys, k)
	}
	if discoveredKeySetHits(ignored, keys) {
		if existing, err := s.GetDiscoveredByIdentityKey(ctx, identity); err == nil && existing != nil {
			return existing.ID, nil
		}
		// Смежный ignored (другой identity_key) — не плодим new.
		return 0, nil
	}
	now := time.Now().UTC()
	var sys *string
	if sn := strings.TrimSpace(sysName); sn != "" {
		sys = &sn
	}
	mgmt := host
	proto := DiscoveredProtocolSNMPScan
	err = s.pool.QueryRow(ctx, `
		INSERT INTO discovered_devices (
			identity_key, remote_sys_name, remote_chassis_id, remote_mgmt_addr,
			first_seen_from_device_id, first_seen_if_index,
			last_seen_from_device_id, last_seen_if_index, last_protocol,
			status, last_seen_at, created_at, updated_at
		) VALUES ($1,$2,NULL,$3,NULL,NULL,NULL,NULL,$4,'new',$5,$5,$5)
		ON CONFLICT (identity_key) DO UPDATE SET
			remote_sys_name = COALESCE(EXCLUDED.remote_sys_name, discovered_devices.remote_sys_name),
			remote_mgmt_addr = COALESCE(EXCLUDED.remote_mgmt_addr, discovered_devices.remote_mgmt_addr),
			last_protocol = EXCLUDED.last_protocol,
			last_seen_at = EXCLUDED.last_seen_at,
			updated_at = EXCLUDED.updated_at
		RETURNING id`,
		identity, sys, mgmt, proto, now,
	).Scan(&id)
	return id, err
}

func nullIfEmpty(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

func (s *Store) ListDiscovered(ctx context.Context, status string) ([]DiscoveredDevice, error) {
	status = strings.TrimSpace(strings.ToLower(status))
	q := `
		SELECT d.id, d.identity_key, d.remote_sys_name, d.remote_chassis_id, d.remote_mgmt_addr,
			d.first_seen_from_device_id, d.first_seen_if_index,
			d.last_seen_from_device_id, d.last_seen_if_index, d.last_protocol,
			d.status, d.promoted_device_id, d.last_seen_at, d.created_at, d.updated_at,
			src.name
		FROM discovered_devices d
		LEFT JOIN devices src ON src.id = d.last_seen_from_device_id`
	args := []interface{}{}
	if status != "" && status != "all" {
		q += ` WHERE d.status = $1`
		args = append(args, status)
	}
	q += ` ORDER BY d.last_seen_at DESC, d.id DESC`

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DiscoveredDevice
	for rows.Next() {
		var d DiscoveredDevice
		if err := rows.Scan(
			&d.ID, &d.IdentityKey, &d.RemoteSysName, &d.RemoteChassisID, &d.RemoteMgmtAddr,
			&d.FirstSeenFromDeviceID, &d.FirstSeenIfIndex,
			&d.LastSeenFromDeviceID, &d.LastSeenIfIndex, &d.LastProtocol,
			&d.Status, &d.PromotedDeviceID, &d.LastSeenAt, &d.CreatedAt, &d.UpdatedAt,
			&d.SeenFromName,
		); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	devices, err := s.ListDevices(ctx)
	if err != nil {
		return nil, err
	}
	return hideDiscoveredAlreadyInInventory(out, devices), nil
}

func (s *Store) GetDiscovered(ctx context.Context, id int64) (*DiscoveredDevice, error) {
	var d DiscoveredDevice
	err := s.pool.QueryRow(ctx, `
		SELECT id, identity_key, remote_sys_name, remote_chassis_id, remote_mgmt_addr,
			first_seen_from_device_id, first_seen_if_index,
			last_seen_from_device_id, last_seen_if_index, last_protocol,
			status, promoted_device_id, last_seen_at, created_at, updated_at
		FROM discovered_devices WHERE id = $1`, id).Scan(
		&d.ID, &d.IdentityKey, &d.RemoteSysName, &d.RemoteChassisID, &d.RemoteMgmtAddr,
		&d.FirstSeenFromDeviceID, &d.FirstSeenIfIndex,
		&d.LastSeenFromDeviceID, &d.LastSeenIfIndex, &d.LastProtocol,
		&d.Status, &d.PromotedDeviceID, &d.LastSeenAt, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &d, nil
}

// GetDiscoveredByIdentityKey — точечный поиск по identity_key (scan / promote).
func (s *Store) GetDiscoveredByIdentityKey(ctx context.Context, identity string) (*DiscoveredDevice, error) {
	identity = strings.TrimSpace(identity)
	if identity == "" {
		return nil, nil
	}
	var d DiscoveredDevice
	err := s.pool.QueryRow(ctx, `
		SELECT id, identity_key, remote_sys_name, remote_chassis_id, remote_mgmt_addr,
			first_seen_from_device_id, first_seen_if_index,
			last_seen_from_device_id, last_seen_if_index, last_protocol,
			status, promoted_device_id, last_seen_at, created_at, updated_at
		FROM discovered_devices WHERE identity_key = $1`, identity).Scan(
		&d.ID, &d.IdentityKey, &d.RemoteSysName, &d.RemoteChassisID, &d.RemoteMgmtAddr,
		&d.FirstSeenFromDeviceID, &d.FirstSeenIfIndex,
		&d.LastSeenFromDeviceID, &d.LastSeenIfIndex, &d.LastProtocol,
		&d.Status, &d.PromotedDeviceID, &d.LastSeenAt, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &d, nil
}

func (s *Store) SetDiscoveredStatus(ctx context.Context, id int64, status string, promotedDeviceID *int64) error {
	status = strings.TrimSpace(strings.ToLower(status))
	switch status {
	case DiscoveredStatusNew, DiscoveredStatusIgnored, DiscoveredStatusAdded:
	default:
		return fmt.Errorf("неверный status: %s", status)
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE discovered_devices SET
			status = $2,
			promoted_device_id = COALESCE($3, promoted_device_id),
			updated_at = now()
		WHERE id = $1`, id, status, promotedDeviceID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrDeviceNotFound
	}
	return nil
}

// ReopenDiscovered возвращает кандидата в status=new (повторное добавление после удаления узла / отмены).
func (s *Store) ReopenDiscovered(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE discovered_devices SET
			status = $2,
			promoted_device_id = NULL,
			updated_at = now()
		WHERE id = $1`, id, DiscoveredStatusNew)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrDeviceNotFound
	}
	return nil
}

// HealOrphanDiscoveredAdded сбрасывает added без существующего узла.
func (s *Store) HealOrphanDiscoveredAdded(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE discovered_devices
		SET status = $1,
		    promoted_device_id = NULL,
		    updated_at = now()
		WHERE status = $2
		  AND (
		    promoted_device_id IS NULL
		    OR NOT EXISTS (SELECT 1 FROM devices d WHERE d.id = discovered_devices.promoted_device_id)
		  )`, DiscoveredStatusNew, DiscoveredStatusAdded)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

type inventoryIdentityIndex struct {
	byMAC  map[string]int64
	byHost map[string]int64
}

func buildInventoryIdentityIndex(devices []models.Device) inventoryIdentityIndex {
	macHits := map[string]map[int64]struct{}{}
	hostHits := map[string]map[int64]struct{}{}
	add := func(m map[string]map[int64]struct{}, key string, id int64) {
		key = strings.TrimSpace(key)
		if key == "" {
			return
		}
		set := m[key]
		if set == nil {
			set = map[int64]struct{}{}
			m[key] = set
		}
		set[id] = struct{}{}
	}
	for _, d := range devices {
		host := strings.ToLower(strings.TrimSpace(d.Host))
		if host != "" {
			add(hostHits, host, d.ID)
		}
		if mac, ok := NormalizeMACQuery(derefStr(d.ChassisMAC)); ok {
			h := macHexDigits(mac)
			if len(h) == 12 {
				add(macHits, h, d.ID)
			}
		}
	}
	uniq := func(hits map[string]map[int64]struct{}) map[string]int64 {
		out := make(map[string]int64, len(hits))
		for k, ids := range hits {
			if len(ids) != 1 {
				continue
			}
			for id := range ids {
				out[k] = id
			}
		}
		return out
	}
	return inventoryIdentityIndex{byMAC: uniq(macHits), byHost: uniq(hostHits)}
}

// matchDiscoveredToInventory — узел уже в inventory по полному MAC или IP/host.
func matchDiscoveredToInventory(idx inventoryIdentityIndex, d DiscoveredDevice) (int64, bool) {
	seenMAC := map[string]struct{}{}
	tryMAC := func(raw string) (int64, bool) {
		mac, ok := NormalizeMACQuery(raw)
		if !ok {
			return 0, false
		}
		h := macHexDigits(mac)
		if len(h) != 12 {
			return 0, false
		}
		if _, dup := seenMAC[h]; dup {
			return 0, false
		}
		seenMAC[h] = struct{}{}
		id, ok := idx.byMAC[h]
		return id, ok
	}
	if id, ok := tryMAC(DiscoveredChassisMAC(&d)); ok {
		return id, true
	}
	if id, ok := tryMAC(derefStr(d.RemoteChassisID)); ok {
		return id, true
	}

	seenHost := map[string]struct{}{}
	tryHost := func(raw string) (int64, bool) {
		h := strings.ToLower(strings.TrimSpace(raw))
		if h == "" {
			return 0, false
		}
		if _, dup := seenHost[h]; dup {
			return 0, false
		}
		seenHost[h] = struct{}{}
		id, ok := idx.byHost[h]
		return id, ok
	}
	if id, ok := tryHost(derefStr(d.RemoteMgmtAddr)); ok {
		return id, true
	}
	key := strings.ToLower(strings.TrimSpace(d.IdentityKey))
	if strings.HasPrefix(key, "addr:") {
		if id, ok := tryHost(strings.TrimPrefix(key, "addr:")); ok {
			return id, true
		}
	}
	return 0, false
}

func hideDiscoveredAlreadyInInventory(list []DiscoveredDevice, devices []models.Device) []DiscoveredDevice {
	idx := buildInventoryIdentityIndex(devices)
	out := make([]DiscoveredDevice, 0, len(list))
	for _, d := range list {
		if d.Status == DiscoveredStatusNew {
			if _, ok := matchDiscoveredToInventory(idx, d); ok {
				continue
			}
		}
		out = append(out, d)
	}
	return out
}

// HealDiscoveredAlreadyInInventory помечает new-кандидатов как added, если MAC/IP уже в Узлах
// или слабый LLDP совпал с единственным inventory в FDB на том же порту.
func (s *Store) HealDiscoveredAlreadyInInventory(ctx context.Context) (int64, error) {
	devices, err := s.ListDevices(ctx)
	if err != nil {
		return 0, err
	}
	return s.markDiscoveredAlreadyInInventory(ctx, devices)
}

func (s *Store) markDiscoveredAlreadyInInventory(ctx context.Context, devices []models.Device) (int64, error) {
	idx := buildInventoryIdentityIndex(devices)
	rows, err := s.pool.Query(ctx, `
		SELECT id, identity_key, remote_sys_name, remote_chassis_id, remote_mgmt_addr, status,
			first_seen_from_device_id, first_seen_if_index,
			last_seen_from_device_id, last_seen_if_index
		FROM discovered_devices
		WHERE status = $1`, DiscoveredStatusNew)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var hits []struct {
		id       int64
		deviceID int64
	}
	for rows.Next() {
		var d DiscoveredDevice
		if err := rows.Scan(
			&d.ID, &d.IdentityKey, &d.RemoteSysName, &d.RemoteChassisID, &d.RemoteMgmtAddr, &d.Status,
			&d.FirstSeenFromDeviceID, &d.FirstSeenIfIndex,
			&d.LastSeenFromDeviceID, &d.LastSeenIfIndex,
		); err != nil {
			return 0, err
		}
		if deviceID, ok := matchDiscoveredToInventory(idx, d); ok {
			hits = append(hits, struct {
				id       int64
				deviceID int64
			}{d.ID, deviceID})
			continue
		}
		if deviceID, ok, err := s.MatchDiscoveredViaPortFDB(ctx, d); err != nil {
			return 0, err
		} else if ok {
			hits = append(hits, struct {
				id       int64
				deviceID int64
			}{d.ID, deviceID})
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	var n int64
	for _, h := range hits {
		id := h.deviceID
		if err := s.SetDiscoveredStatus(ctx, h.id, DiscoveredStatusAdded, &id); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// MatchDiscoveredViaPortFDB — слабый кандидат (без MAC/IP) + ровно один inventory в FDB на порту.
func (s *Store) MatchDiscoveredViaPortFDB(ctx context.Context, d DiscoveredDevice) (int64, bool, error) {
	if !discoveredLacksStrongIdentity(d) {
		return 0, false, nil
	}
	switchID, ifIndex, ok := discoveredPortAnchor(d)
	if !ok {
		return 0, false, nil
	}
	hint, err := s.UniqueInventoryOnPort(ctx, switchID, ifIndex)
	if err != nil || hint == nil {
		return 0, false, err
	}
	return hint.DeviceID, true, nil
}

// AttachLikelyInventoryHints заполняет likely_device_* для new-кандидатов (UI «Это тот же узел»).
func (s *Store) AttachLikelyInventoryHints(ctx context.Context, list []DiscoveredDevice) error {
	for i := range list {
		d := &list[i]
		if d.Status != DiscoveredStatusNew || !discoveredLacksStrongIdentity(*d) {
			continue
		}
		switchID, ifIndex, ok := discoveredPortAnchor(*d)
		if !ok {
			continue
		}
		hint, err := s.UniqueInventoryOnPort(ctx, switchID, ifIndex)
		if err != nil {
			return err
		}
		if hint == nil {
			continue
		}
		id := hint.DeviceID
		d.LikelyDeviceID = &id
		name := hint.Name
		d.LikelyDeviceName = &name
	}
	return nil
}

// LinkDiscoveredToDevice помечает кандидата added с существующим узлом (без CreateDevice).
func (s *Store) LinkDiscoveredToDevice(ctx context.Context, discoveredID, deviceID int64) error {
	if discoveredID <= 0 || deviceID <= 0 {
		return fmt.Errorf("неверный id")
	}
	d, err := s.GetDiscovered(ctx, discoveredID)
	if err != nil {
		return err
	}
	if d == nil {
		return ErrDeviceNotFound
	}
	dev, err := s.GetDevice(ctx, deviceID)
	if err != nil {
		return err
	}
	if dev == nil {
		return ErrDeviceNotFound
	}
	return s.SetDiscoveredStatus(ctx, discoveredID, DiscoveredStatusAdded, &deviceID)
}

// SuggestDiscoveredHost — предпочтительный host для promote (mgmt addr или sys_name).
func SuggestDiscoveredHost(d *DiscoveredDevice) string {
	if d == nil {
		return ""
	}
	if a := derefStr(d.RemoteMgmtAddr); a != "" {
		return a
	}
	if n := derefStr(d.RemoteSysName); n != "" {
		// MAC в sys_name не годится как SNMP host
		if _, ok := NormalizeMACQuery(n); ok {
			return ""
		}
		return n
	}
	return ""
}

func SuggestDiscoveredName(d *DiscoveredDevice) string {
	if d == nil {
		return ""
	}
	if n := derefStr(d.RemoteSysName); n != "" {
		return n
	}
	if a := derefStr(d.RemoteMgmtAddr); a != "" {
		return a
	}
	return d.IdentityKey
}

// DiscoveredChassisMAC — MAC из chassis LLDP кандидата (для записи в devices при promote).
func DiscoveredChassisMAC(d *DiscoveredDevice) string {
	if d == nil {
		return ""
	}
	if mac, ok := NormalizeMACQuery(derefStr(d.RemoteChassisID)); ok {
		return mac
	}
	// identity_key вида chassis:b47af1ddc444
	key := strings.TrimSpace(strings.ToLower(d.IdentityKey))
	if strings.HasPrefix(key, "chassis:") {
		if mac, ok := NormalizeMACQuery(strings.TrimPrefix(key, "chassis:")); ok {
			return mac
		}
	}
	return ""
}

// FilterUnknownNeighbors — для тестов/хелперов: оставить только неизвестных.
func FilterUnknownNeighbors(devices []models.Device, neighbors []PortNeighbor) []PortNeighbor {
	idx := buildDeviceNameIndex(devices)
	var out []PortNeighbor
	for _, nb := range neighbors {
		if _, ok := ShouldOfferDiscovered(idx, nb); ok {
			out = append(out, nb)
		}
	}
	return out
}
