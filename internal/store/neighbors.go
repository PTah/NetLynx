package store

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// NeighborStaleTTL — после этого без успешного наблюдения запись удаляется.
const NeighborStaleTTL = 2 * time.Hour

// NeighborProtocolFDB — сосед, закреплённый с порта по MAC из FDB (не LLDP/CDP).
// Poller не трогает этот protocol при upsert lldp/cdp, поэтому ребро живёт на топологии.
const NeighborProtocolFDB = "fdb"

type PortNeighbor struct {
	DeviceID        int64     `json:"device_id"`
	IfIndex         int       `json:"if_index"`
	RemIndex        int       `json:"rem_index"`
	Protocol        string    `json:"protocol"`
	RemoteSysName   *string   `json:"remote_sys_name,omitempty"`
	RemotePortID    *string   `json:"remote_port_id,omitempty"`
	RemoteChassisID *string   `json:"remote_chassis_id,omitempty"`
	RemoteMgmtAddr  *string   `json:"remote_mgmt_addr,omitempty"`
	RemoteDeviceID  *int64    `json:"remote_device_id,omitempty"`
	Stale           bool      `json:"stale"`
	LastSeenAt      time.Time `json:"last_seen_at"`
}

// UpsertPortNeighbors пишет актуальный снимок протокола для устройства.
// Невиденные в этом опросе помечаются stale; старше NeighborStaleTTL удаляются.
// Дубликаты с тем же remote identity (MikroTik и др. меняют rem_index на каждом опросе) схлопываются.
// При пустом списке (успешный walk без соседей) все текущие записи протокола становятся stale.
func (s *Store) UpsertPortNeighbors(ctx context.Context, deviceID int64, protocol string, neighbors []PortNeighbor, at time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	seen := make(map[string]struct{}, len(neighbors))
	for _, n := range neighbors {
		if n.IfIndex <= 0 {
			continue
		}
		ri := n.RemIndex
		if ri <= 0 {
			ri = 1
		}
		key := neighborKey(n.IfIndex, ri)
		seen[key] = struct{}{}
		if _, err := tx.Exec(ctx, `
			INSERT INTO port_neighbors (
				device_id, if_index, rem_index, protocol,
				remote_sys_name, remote_port_id, remote_chassis_id, remote_mgmt_addr,
				stale, last_seen_at, updated_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,false,$9,$9)
			ON CONFLICT (device_id, if_index, protocol, rem_index) DO UPDATE SET
				remote_sys_name = EXCLUDED.remote_sys_name,
				remote_port_id = EXCLUDED.remote_port_id,
				remote_chassis_id = EXCLUDED.remote_chassis_id,
				remote_mgmt_addr = EXCLUDED.remote_mgmt_addr,
				stale = false,
				last_seen_at = EXCLUDED.last_seen_at,
				updated_at = EXCLUDED.updated_at`,
			deviceID, n.IfIndex, ri, protocol,
			n.RemoteSysName, n.RemotePortID, n.RemoteChassisID, n.RemoteMgmtAddr, at); err != nil {
			return err
		}
	}

	rows, err := tx.Query(ctx, `
		SELECT if_index, rem_index FROM port_neighbors
		WHERE device_id = $1 AND protocol = $2`, deviceID, protocol)
	if err != nil {
		return err
	}
	var toStale [][2]int
	for rows.Next() {
		var ifi, ri int
		if err := rows.Scan(&ifi, &ri); err != nil {
			rows.Close()
			return err
		}
		if _, ok := seen[neighborKey(ifi, ri)]; !ok {
			toStale = append(toStale, [2]int{ifi, ri})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, p := range toStale {
		if _, err := tx.Exec(ctx, `
			UPDATE port_neighbors SET stale = true, updated_at = $4
			WHERE device_id = $1 AND if_index = $2 AND rem_index = $3 AND protocol = $5`,
			deviceID, p[0], p[1], at, protocol); err != nil {
			return err
		}
	}

	cutoff := at.Add(-NeighborStaleTTL)
	if _, err := tx.Exec(ctx, `
		DELETE FROM port_neighbors
		WHERE device_id = $1 AND protocol = $2 AND stale = true AND last_seen_at < $3`,
		deviceID, protocol, cutoff); err != nil {
		return err
	}

	// MikroTik и др.: rem_index (часто timeMark) меняется → копим stale с тем же sysName/port.
	// Оставляем одну запись на identity: предпочитаем live, затем свежий last_seen, затем больший rem_index.
	if _, err := tx.Exec(ctx, `
		DELETE FROM port_neighbors AS pn
		WHERE pn.device_id = $1 AND pn.protocol = $2
		AND EXISTS (
			SELECT 1 FROM port_neighbors AS keep
			WHERE keep.device_id = pn.device_id
				AND keep.if_index = pn.if_index
				AND keep.protocol = pn.protocol
				AND COALESCE(keep.remote_sys_name, '') = COALESCE(pn.remote_sys_name, '')
				AND COALESCE(keep.remote_port_id, '') = COALESCE(pn.remote_port_id, '')
				AND COALESCE(keep.remote_chassis_id, '') = COALESCE(pn.remote_chassis_id, '')
				AND (
					(keep.stale = false AND pn.stale = true)
					OR (
						keep.stale = pn.stale
						AND (
							keep.last_seen_at > pn.last_seen_at
							OR (keep.last_seen_at = pn.last_seen_at AND keep.rem_index > pn.rem_index)
						)
					)
				)
		)`, deviceID, protocol); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func neighborKey(ifIndex, remIndex int) string {
	return strconv.Itoa(ifIndex) + ":" + strconv.Itoa(remIndex)
}

func neighborIdentityKey(n PortNeighbor) string {
	return strings.Join([]string{
		strconv.FormatInt(n.DeviceID, 10),
		strconv.Itoa(n.IfIndex),
		n.Protocol,
		derefNeighborStr(n.RemoteSysName),
		derefNeighborStr(n.RemotePortID),
		derefNeighborStr(n.RemoteChassisID),
	}, "\x00")
}

func derefNeighborStr(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}

// betterNeighbor выбирает более актуальную запись среди дублей одной identity.
func betterNeighbor(a, b PortNeighbor) PortNeighbor {
	if a.Stale != b.Stale {
		if !a.Stale {
			return a
		}
		return b
	}
	if a.LastSeenAt.After(b.LastSeenAt) {
		return a
	}
	if b.LastSeenAt.After(a.LastSeenAt) {
		return b
	}
	if a.RemIndex >= b.RemIndex {
		return a
	}
	return b
}

// CollapsePortNeighborsByIdentity оставляет одну запись на (device, if, protocol, remote*).
func CollapsePortNeighborsByIdentity(in []PortNeighbor) []PortNeighbor {
	if len(in) <= 1 {
		return in
	}
	best := make(map[string]PortNeighbor, len(in))
	order := make([]string, 0, len(in))
	for _, n := range in {
		k := neighborIdentityKey(n)
		prev, ok := best[k]
		if !ok {
			best[k] = n
			order = append(order, k)
			continue
		}
		best[k] = betterNeighbor(prev, n)
	}
	out := make([]PortNeighbor, 0, len(order))
	for _, k := range order {
		out = append(out, best[k])
	}
	return out
}

func (s *Store) ListPortNeighbors(ctx context.Context, deviceID int64) ([]PortNeighbor, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT device_id, if_index, rem_index, protocol,
			remote_sys_name, remote_port_id, remote_chassis_id, remote_mgmt_addr,
			stale, last_seen_at
		FROM port_neighbors WHERE device_id = $1
		ORDER BY if_index, protocol, rem_index`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PortNeighbor
	for rows.Next() {
		var n PortNeighbor
		if err := rows.Scan(
			&n.DeviceID, &n.IfIndex, &n.RemIndex, &n.Protocol,
			&n.RemoteSysName, &n.RemotePortID, &n.RemoteChassisID, &n.RemoteMgmtAddr,
			&n.Stale, &n.LastSeenAt,
		); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return CollapsePortNeighborsByIdentity(out), nil
}

func (s *Store) ListAllPortNeighbors(ctx context.Context) ([]PortNeighbor, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT device_id, if_index, rem_index, protocol,
			remote_sys_name, remote_port_id, remote_chassis_id, remote_mgmt_addr,
			stale, last_seen_at
		FROM port_neighbors
		ORDER BY device_id, if_index, protocol, rem_index`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PortNeighbor
	for rows.Next() {
		var n PortNeighbor
		if err := rows.Scan(
			&n.DeviceID, &n.IfIndex, &n.RemIndex, &n.Protocol,
			&n.RemoteSysName, &n.RemotePortID, &n.RemoteChassisID, &n.RemoteMgmtAddr,
			&n.Stale, &n.LastSeenAt,
		); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return CollapsePortNeighborsByIdentity(out), nil
}

// UpsertFDBTopologyNeighbor закрепляет соседа protocol=fdb на порту (MAC → chassis).
// Не помечает stale другие FDB-записи этого устройства.
func (s *Store) UpsertFDBTopologyNeighbor(ctx context.Context, localDeviceID int64, ifIndex int, mac string, mgmtAddr, sysName *string, at time.Time) error {
	if localDeviceID <= 0 || ifIndex <= 0 {
		return fmt.Errorf("device_id and if_index must be positive")
	}
	macNorm, ok := FormatFullMAC(mac)
	if !ok {
		return fmt.Errorf("нужен полный MAC")
	}
	hex := macHexDigits(macNorm)
	if at.IsZero() {
		at = time.Now().UTC()
	}
	var mgmt any
	if mgmtAddr != nil {
		if t := strings.TrimSpace(*mgmtAddr); t != "" {
			mgmt = t
		}
	}
	var sys any
	if sysName != nil {
		if t := strings.TrimSpace(*sysName); t != "" {
			sys = t
		}
	}

	var remIndex int
	err := s.pool.QueryRow(ctx, `
		SELECT rem_index FROM port_neighbors
		WHERE device_id = $1 AND if_index = $2 AND protocol = $3
		  AND lower(replace(replace(COALESCE(remote_chassis_id, ''), ':', ''), '-', '')) = $4
		LIMIT 1`,
		localDeviceID, ifIndex, NeighborProtocolFDB, hex,
	).Scan(&remIndex)
	if errors.Is(err, pgx.ErrNoRows) {
		err = s.pool.QueryRow(ctx, `
			SELECT COALESCE(MAX(rem_index), 0) + 1 FROM port_neighbors
			WHERE device_id = $1 AND if_index = $2 AND protocol = $3`,
			localDeviceID, ifIndex, NeighborProtocolFDB,
		).Scan(&remIndex)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO port_neighbors (
			device_id, if_index, rem_index, protocol,
			remote_sys_name, remote_port_id, remote_chassis_id, remote_mgmt_addr,
			stale, last_seen_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,NULL,$6,$7,false,$8,$8)
		ON CONFLICT (device_id, if_index, protocol, rem_index) DO UPDATE SET
			remote_sys_name = COALESCE(EXCLUDED.remote_sys_name, port_neighbors.remote_sys_name),
			remote_chassis_id = EXCLUDED.remote_chassis_id,
			remote_mgmt_addr = COALESCE(EXCLUDED.remote_mgmt_addr, port_neighbors.remote_mgmt_addr),
			stale = false,
			last_seen_at = EXCLUDED.last_seen_at,
			updated_at = EXCLUDED.updated_at`,
		localDeviceID, ifIndex, remIndex, NeighborProtocolFDB,
		sys, macNorm, mgmt, at,
	)
	return err
}

// ChassisEndpoint — узел inventory с известным chassis MAC (для FDB→топология).
type ChassisEndpoint struct {
	ID       int64
	Name     string
	Host     string
	MAC      string // aa:bb:…
	Hex      string // 12 hex
	Category string // device_category (ap, switch, …)
}

// ListChassisMACIndex: hex MAC → endpoint (только узлы с chassis_mac).
func (s *Store) ListChassisMACIndex(ctx context.Context) (map[string]ChassisEndpoint, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, COALESCE(host, ''), chassis_mac, COALESCE(device_category, '')
		FROM devices
		WHERE chassis_mac IS NOT NULL AND btrim(chassis_mac) <> ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]ChassisEndpoint)
	for rows.Next() {
		var id int64
		var name, host, raw, cat string
		if err := rows.Scan(&id, &name, &host, &raw, &cat); err != nil {
			return nil, err
		}
		mac, ok := FormatFullMAC(raw)
		if !ok {
			continue
		}
		hex := macHexDigits(mac)
		if len(hex) != 12 {
			continue
		}
		out[hex] = ChassisEndpoint{ID: id, Name: name, Host: host, MAC: mac, Hex: hex, Category: NormalizeDeviceCategory(cat)}
	}
	return out, rows.Err()
}

const fdbTopoMaxMACsOnPort = 10 // access: больше — шум/trunk-like
const fdbTopoMaxAPsOnTrunk = 1  // только uplink одной AP на trunk
const fdbTopoSiblingTrunkAPFlood = 4 // >=4 single-AP trunk на свитче — core-like; edge multi-AP (48 #7) обычно 3
const fdbTopoMinNonInventoryMACsOnAPTrunk = 2
const fdbTopoMinMACsQuietAPTrunk = 8
const fdbAPLinkMinTrunkScore = 800 // ниже — только ghost; real AP uplink обычно macCount >> 10

// fdbAPLinkScore — выше = правдоподобнее uplink AP.
// UniFi uplink часто trunk с 1–10 MAC; core XG flood — сотни MAC на trunk → ghost.
func fdbAPLinkScore(role string, macCount, apCount, invCount, maxSibling int) int {
	role = strings.ToLower(strings.TrimSpace(role))
	switch role {
	case "access":
		return 1000000 + macCount
	case "trunk":
		if apCount != fdbTopoMaxAPsOnTrunk {
			return -1
		}
		if !trunkAPUplinkLikely(macCount, apCount, invCount, maxSibling) {
			return -1
		}
		if macCount <= fdbTopoMaxMACsOnPort {
			return 1000000 + (fdbTopoMaxMACsOnPort - macCount)
		}
		nonInv := macCount - invCount
		if nonInv < 0 {
			nonInv = 0
		}
		return macCount*100 + nonInv*50 - maxSibling*5
	default:
		return -1
	}
}

type fdbPortInventoryStats struct {
	apDeviceCount     map[int]int // уникальных AP (device_category=ap) на порту
	inventoryMACCount map[int]int // все chassis_mac inventory на порту
}

// inventoryStatsByPort — inventory MAC на порту, сгруппированные по типу узла.
func inventoryStatsByPort(entries map[string]FDBLearnedEntry, chassis map[string]ChassisEndpoint) fdbPortInventoryStats {
	apDevices := make(map[int]map[int64]struct{})
	inventoryMAC := make(map[int]int)
	for mac, ent := range entries {
		if ent.IfIndex <= 0 {
			continue
		}
		macNorm, ok := FormatFullMAC(mac)
		if !ok {
			continue
		}
		ep, ok := chassis[macHexDigits(macNorm)]
		if !ok {
			continue
		}
		ifIndex := ent.IfIndex
		inventoryMAC[ifIndex]++
		if NormalizeDeviceCategory(ep.Category) != DeviceCategoryAP {
			continue
		}
		if apDevices[ifIndex] == nil {
			apDevices[ifIndex] = make(map[int64]struct{})
		}
		apDevices[ifIndex][ep.ID] = struct{}{}
	}
	apDeviceCount := make(map[int]int, len(apDevices))
	for ifIndex, ids := range apDevices {
		apDeviceCount[ifIndex] = len(ids)
	}
	return fdbPortInventoryStats{apDeviceCount: apDeviceCount, inventoryMACCount: inventoryMAC}
}

// maxSiblingTrunkAPCount — сколько других trunk-портов с ровно одной AP (uplink-подобных).
// Порты uplink-flood (2+ AP на trunk) не считаем — иначе edge-порты AP блокируются.
func maxSiblingTrunkAPCount(apDeviceCount map[int]int, portRole map[int]string, ifIndex int) int {
	n := 0
	for other, apN := range apDeviceCount {
		if other == ifIndex {
			continue
		}
		if strings.ToLower(strings.TrimSpace(portRole[other])) != "trunk" {
			continue
		}
		if apN != fdbTopoMaxAPsOnTrunk {
			continue
		}
		n++
	}
	return n
}

// trunkAPUplinkLikely — trunk с одной AP: uplink edge (мало MAC) vs VLAN-flood на core (XG).
func trunkAPUplinkLikely(portMACCount, apDeviceCount, inventoryMACCount, maxSiblingTrunkAP int) bool {
	if apDeviceCount != fdbTopoMaxAPsOnTrunk {
		return false
	}
	if portMACCount <= 0 {
		return false
	}
	// Core/distribution (много AP на соседних trunk): FDB AP на trunk — ghost.
	if maxSiblingTrunkAP >= fdbTopoSiblingTrunkAPFlood {
		return false
	}
	if portMACCount <= fdbTopoMaxMACsOnPort {
		return true
	}
	nonInventory := portMACCount - inventoryMACCount
	if nonInventory >= fdbTopoMinNonInventoryMACsOnAPTrunk {
		return portMACCount <= fdbTopoMinMACsQuietAPTrunk*4
	}
	return portMACCount >= fdbTopoMinMACsQuietAPTrunk && portMACCount <= fdbTopoMinMACsQuietAPTrunk*4
}

// fdbTopoPortEligible — порт подходит для FDB→топология (access или trunk к AP).
func fdbTopoPortEligible(role string, portMACCount, apDeviceCount, inventoryMACCount, maxSiblingTrunkAP int) bool {
	role = strings.ToLower(strings.TrimSpace(role))
	if role == "ignore" || portMACCount <= 0 {
		return false
	}
	switch role {
	case "access":
		return portMACCount <= fdbTopoMaxMACsOnPort
	case "trunk":
		return trunkAPUplinkLikely(portMACCount, apDeviceCount, inventoryMACCount, maxSiblingTrunkAP)
	default:
		return portMACCount <= fdbTopoMaxMACsOnPort
	}
}

// fdbTopoLinkEligible — можно ли строить линк к конкретному inventory MAC.
func fdbTopoLinkEligible(role, remoteCategory string) bool {
	role = strings.ToLower(strings.TrimSpace(role))
	switch role {
	case "access":
		return true
	case "trunk":
		return NormalizeDeviceCategory(remoteCategory) == DeviceCategoryAP
	default:
		return NormalizeDeviceCategory(remoteCategory) == DeviceCategoryAP
	}
}

// SyncFDBTopologyNeighbors — FDB MAC → protocol=fdb на порту свитча.
// Access: любой inventory; trunk: только device_category=ap (UniFi uplink). Линки sticky при offline.
func (s *Store) SyncFDBTopologyNeighbors(
	ctx context.Context,
	switchID int64,
	entries map[string]FDBLearnedEntry,
	portMACCount map[int]int,
	portRole map[int]string,
	chassis map[string]ChassisEndpoint,
	at time.Time,
) (linked int, err error) {
	if switchID <= 0 || len(entries) == 0 || len(chassis) == 0 {
		return 0, nil
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	inv := inventoryStatsByPort(entries, chassis)
	present := make(map[string]int) // hex → ifIndex on this switch (eligible ports)
	for mac, ent := range entries {
		ifIndex := ent.IfIndex
		if ifIndex <= 0 {
			continue
		}
		sibling := maxSiblingTrunkAPCount(inv.apDeviceCount, portRole, ifIndex)
		if !fdbTopoPortEligible(portRole[ifIndex], portMACCount[ifIndex], inv.apDeviceCount[ifIndex], inv.inventoryMACCount[ifIndex], sibling) {
			continue
		}
		macNorm, ok := FormatFullMAC(mac)
		if !ok {
			continue
		}
		hex := macHexDigits(macNorm)
		ep, ok := chassis[hex]
		if !ok || ep.ID == switchID {
			continue
		}
		if !fdbTopoLinkEligible(portRole[ifIndex], ep.Category) {
			continue
		}
		if fdbAPLinkScore(portRole[ifIndex], portMACCount[ifIndex], inv.apDeviceCount[ifIndex], inv.inventoryMACCount[ifIndex], sibling) < fdbAPLinkMinTrunkScore &&
			strings.ToLower(strings.TrimSpace(portRole[ifIndex])) == "trunk" {
			continue
		}
		present[hex] = ifIndex
		var mgmt, sys *string
		if h := strings.TrimSpace(ep.Host); h != "" {
			mgmt = &h
		}
		if n := strings.TrimSpace(ep.Name); n != "" {
			sys = &n
		}
		if err := s.UpsertFDBTopologyNeighbor(ctx, switchID, ifIndex, macNorm, mgmt, sys, at); err != nil {
			return linked, err
		}
		linked++
	}
	// Stale: переезд MAC на другой порт; FDB-линки на trunk/uplink и прочих не-access.
	rows, err := s.pool.Query(ctx, `
		SELECT if_index, rem_index, remote_chassis_id FROM port_neighbors
		WHERE device_id = $1 AND protocol = $2 AND stale = false`,
		switchID, NeighborProtocolFDB)
	if err != nil {
		return linked, err
	}
	defer rows.Close()
	type gone struct{ ifi, ri int }
	toStale := make([]gone, 0)
	staleKey := func(ifi, ri int) string { return fmt.Sprintf("%d:%d", ifi, ri) }
	seenStale := make(map[string]struct{})
	addStale := func(ifi, ri int) {
		k := staleKey(ifi, ri)
		if _, ok := seenStale[k]; ok {
			return
		}
		seenStale[k] = struct{}{}
		toStale = append(toStale, gone{ifi, ri})
	}
	for rows.Next() {
		var ifi, ri int
		var ch *string
		if err := rows.Scan(&ifi, &ri, &ch); err != nil {
			return linked, err
		}
		sibling := maxSiblingTrunkAPCount(inv.apDeviceCount, portRole, ifi)
		if !fdbTopoPortEligible(portRole[ifi], portMACCount[ifi], inv.apDeviceCount[ifi], inv.inventoryMACCount[ifi], sibling) {
			hex := ""
			if ch != nil {
				if m, ok := FormatFullMAC(*ch); ok {
					hex = macHexDigits(m)
				}
			}
			isAPLink := false
			if hex != "" {
				if ep, ok := chassis[hex]; ok && NormalizeDeviceCategory(ep.Category) == DeviceCategoryAP {
					isAPLink = true
				}
			}
			if !isAPLink {
				addStale(ifi, ri)
			}
			continue
		}
		hex := ""
		if ch != nil {
			if m, ok := FormatFullMAC(*ch); ok {
				hex = macHexDigits(m)
			}
		}
		if hex == "" {
			continue
		}
		if ep, ok := chassis[hex]; !ok || !fdbTopoLinkEligible(portRole[ifi], ep.Category) {
			addStale(ifi, ri)
			continue
		}
		curIf, seen := present[hex]
		if !seen {
			continue // offline / ещё не в FDB — sticky
		}
		if curIf != ifi {
			addStale(ifi, ri)
		}
	}
	if err := rows.Err(); err != nil {
		return linked, err
	}
	for _, g := range toStale {
		if _, err := s.pool.Exec(ctx, `
			UPDATE port_neighbors SET stale = true, updated_at = $4
			WHERE device_id = $1 AND if_index = $2 AND rem_index = $3 AND protocol = $5`,
			switchID, g.ifi, g.ri, at, NeighborProtocolFDB); err != nil {
			return linked, err
		}
	}
	cutoff := at.Add(-NeighborStaleTTL)
	_, err = s.pool.Exec(ctx, `
		DELETE FROM port_neighbors
		WHERE device_id = $1 AND protocol = $2 AND stale = true AND last_seen_at < $3`,
		switchID, NeighborProtocolFDB, cutoff)
	if err != nil {
		return linked, err
	}
	if _, err := s.PruneGlobalFDBAPLinks(ctx, at); err != nil {
		return linked, err
	}
	return linked, nil
}

type fdbAPLinkCandidate struct {
	deviceID int64
	ifIndex  int
	remIndex int
	hex      string
	score    int
}

// PruneGlobalFDBAPLinks — одна FDB-связь на inventory MAC (AP, ПК, камеры…); слабые ghost → stale.
func (s *Store) PruneGlobalFDBAPLinks(ctx context.Context, at time.Time) (int, error) {
	return s.PruneGlobalFDBChassisLinks(ctx, at)
}

// PruneGlobalFDBChassisLinks — одна FDB-связь на chassis MAC inventory; ghost на core/trunk → stale.
func (s *Store) PruneGlobalFDBChassisLinks(ctx context.Context, at time.Time) (int, error) {
	if at.IsZero() {
		at = time.Now().UTC()
	}
	chassis, err := s.ListChassisMACIndex(ctx)
	if err != nil {
		return 0, err
	}
	fdbCtx, err := s.LoadFDBPortTopoContext(ctx)
	if err != nil {
		return 0, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT pn.device_id, pn.if_index, pn.rem_index, pn.remote_chassis_id,
			COALESCE(di.port_role, ''), di.cli_port_mode
		FROM port_neighbors pn
		LEFT JOIN device_interfaces di ON di.device_id = pn.device_id AND di.if_index = pn.if_index
		WHERE pn.protocol = $1 AND pn.stale = false AND pn.remote_chassis_id IS NOT NULL`,
		NeighborProtocolFDB)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	byHex := make(map[string][]fdbAPLinkCandidate)
	for rows.Next() {
		var deviceID int64
		var ifIndex, remIndex int
		var chassisRaw *string
		var portRole string
		var cliMode *string
		if err := rows.Scan(&deviceID, &ifIndex, &remIndex, &chassisRaw, &portRole, &cliMode); err != nil {
			return 0, err
		}
		if chassisRaw == nil {
			continue
		}
		mac, ok := FormatFullMAC(*chassisRaw)
		if !ok {
			continue
		}
		hex := macHexDigits(mac)
		ep, ok := chassis[hex]
		if !ok {
			continue
		}
		pcat := NormalizeDeviceCategory(ep.Category)
		if pcat == DeviceCategorySwitch || pcat == DeviceCategoryRouter {
			continue
		}
		role := ResolveInterfacePortRole(portRole, cliMode)
		pk := portKey(deviceID, ifIndex)
		sc := fdbAPLinkScore(role, fdbCtx.MACCount[pk], fdbCtx.APDeviceCount[pk], fdbCtx.InventoryMACCount[pk], fdbCtx.MaxSiblingTrunkAP[pk])
		byHex[hex] = append(byHex[hex], fdbAPLinkCandidate{
			deviceID: deviceID, ifIndex: ifIndex, remIndex: remIndex, hex: hex, score: sc,
		})
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	staled := 0
	for _, cands := range byHex {
		if len(cands) == 0 {
			continue
		}
		hasStrongAccess := false
		for _, c := range cands {
			if c.score >= 1000000 {
				hasStrongAccess = true
				break
			}
		}
		// Без access-кандидата (score tier-1) — только trunk ghosts; не staleим live access вне снимка.
		if !hasStrongAccess {
			continue
		}
		best := cands[0]
		for _, c := range cands[1:] {
			if c.score > best.score {
				best = c
			}
		}
		minScore := fdbAPLinkMinTrunkScore
		if best.score >= 1000000 {
			minScore = 0
		}
		for _, c := range cands {
			staleIt := best.score < minScore ||
				c.score < best.score ||
				(c.score == best.score && (c.deviceID != best.deviceID || c.ifIndex != best.ifIndex || c.remIndex != best.remIndex))
			if !staleIt {
				continue
			}
			if _, err := s.pool.Exec(ctx, `
				UPDATE port_neighbors SET stale = true, updated_at = $4
				WHERE device_id = $1 AND if_index = $2 AND rem_index = $3 AND protocol = $5 AND stale = false`,
				c.deviceID, c.ifIndex, c.remIndex, at, NeighborProtocolFDB); err != nil {
				return staled, err
			}
			staled++
		}
	}
	return staled, nil
}

// RepairAllFDBTopologyLinks: разовый проход по device_fdb_entries → protocol=fdb линки.
func (s *Store) RepairAllFDBTopologyLinks(ctx context.Context) (int, error) {
	chassis, err := s.ListChassisMACIndex(ctx)
	if err != nil {
		return 0, err
	}
	if len(chassis) == 0 {
		return 0, nil
	}
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT device_id FROM device_fdb_entries`)
	if err != nil {
		return 0, err
	}
	var switchIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		switchIDs = append(switchIDs, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	total := 0
	now := time.Now().UTC()
	for _, switchID := range switchIDs {
		fdbRows, err := s.pool.Query(ctx, `
			SELECT mac, if_index FROM device_fdb_entries WHERE device_id = $1`, switchID)
		if err != nil {
			return total, err
		}
		entries := make(map[string]FDBLearnedEntry)
		portMACCount := make(map[int]int)
		for fdbRows.Next() {
			var mac string
			var ifIndex int
			if err := fdbRows.Scan(&mac, &ifIndex); err != nil {
				fdbRows.Close()
				return total, err
			}
			entries[mac] = FDBLearnedEntry{IfIndex: ifIndex}
			portMACCount[ifIndex]++
		}
		fdbRows.Close()
		if err := fdbRows.Err(); err != nil {
			return total, err
		}
		roleRows, err := s.pool.Query(ctx, `
			SELECT if_index, COALESCE(port_role, ''), cli_port_mode
			FROM device_interfaces WHERE device_id = $1`, switchID)
		if err != nil {
			return total, err
		}
		portRole := make(map[int]string)
		for roleRows.Next() {
			var ifIndex int
			var role string
			var cliMode *string
			if err := roleRows.Scan(&ifIndex, &role, &cliMode); err != nil {
				roleRows.Close()
				return total, err
			}
			portRole[ifIndex] = ResolveInterfacePortRole(role, cliMode)
		}
		roleRows.Close()
		if err := roleRows.Err(); err != nil {
			return total, err
		}
		n, err := s.SyncFDBTopologyNeighbors(ctx, switchID, entries, portMACCount, portRole, chassis, now)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}
