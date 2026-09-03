package store

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/models"
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
	now := time.Now().UTC()

	for _, nb := range neighbors {
		identity, offer := ShouldOfferDiscovered(idx, nb)
		if !offer {
			continue
		}
		if err := s.upsertDiscoveredCandidate(ctx, identity, sourceDeviceID, nb, now); err != nil {
			return err
		}
	}
	_, err = s.markDiscoveredAlreadyInInventory(ctx, devices)
	return err
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

// HealDiscoveredAlreadyInInventory помечает new-кандидатов как added, если MAC/IP уже в Узлах.
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
		SELECT id, identity_key, remote_sys_name, remote_chassis_id, remote_mgmt_addr, status
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
		if err := rows.Scan(&d.ID, &d.IdentityKey, &d.RemoteSysName, &d.RemoteChassisID, &d.RemoteMgmtAddr, &d.Status); err != nil {
			return 0, err
		}
		if deviceID, ok := matchDiscoveredToInventory(idx, d); ok {
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
