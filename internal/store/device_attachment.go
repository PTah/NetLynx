package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/models"
)

// attachmentNeighborMaxAge — для оффлайн-оповещений: stale FDB-соседи и недавний FDB.
const attachmentNeighborMaxAge = 7 * 24 * time.Hour

// persistedAttachmentMaxAge — fallback для DEVICE_ONLINE до следующего FDB poll.
const persistedAttachmentMaxAge = 365 * 24 * time.Hour

// DeviceAttachment — где узел «висит» на инфраструктуре (свитч/роутер + порт).
type DeviceAttachment struct {
	ParentID       int64
	ParentName     string
	ParentHost     string
	ParentCategory string // switch | router | …
	PortName       string // if_name или ifIndex
	IfIndex        int    // для persist в devices.attach_*
	Protocol       string // lldp | cdp | manual | fdb
}

func (a DeviceAttachment) ParentKindLabel() string {
	switch NormalizeDeviceCategory(a.ParentCategory) {
	case DeviceCategorySwitch:
		return "коммутаторе"
	case DeviceCategoryRouter:
		return "роутере"
	case DeviceCategoryAP:
		return "точке доступа"
	default:
		return "узле"
	}
}

// FormatRU: «на коммутаторе «X», порт 0/12».
func (a DeviceAttachment) FormatRU() string {
	name := strings.TrimSpace(a.ParentName)
	if name == "" {
		name = strings.TrimSpace(a.ParentHost)
	}
	if name == "" {
		name = fmt.Sprintf("#%d", a.ParentID)
	}
	port := strings.TrimSpace(a.PortName)
	if port != "" {
		return fmt.Sprintf("на %s «%s», порт %s", a.ParentKindLabel(), name, port)
	}
	return fmt.Sprintf("на %s «%s»", a.ParentKindLabel(), name)
}

func attachmentScore(parentCat, targetCat, proto, portRole string, trunkAPOK bool) int {
	score := 0
	switch NormalizeDeviceCategory(parentCat) {
	case DeviceCategorySwitch:
		score += 100
	case DeviceCategoryRouter:
		score += 80
	case DeviceCategoryAP:
		score += 40
	default:
		score += 10
	}
	switch strings.ToLower(strings.TrimSpace(proto)) {
	case "manual":
		score += 30
	case "lldp":
		score += 25
	case "cdp":
		score += 20
	case NeighborProtocolFDB: // "fdb"
		score += 5
	default:
		score += 10
	}
	portRole = strings.ToLower(strings.TrimSpace(portRole))
	targetAP := NormalizeDeviceCategory(targetCat) == DeviceCategoryAP
	fdbAP := targetAP && strings.EqualFold(proto, NeighborProtocolFDB)
	if fdbAP && portRole == "trunk" {
		if trunkAPOK {
			score += 15
		} else {
			score -= 500
		}
	} else {
		switch portRole {
		case "access":
			score += 35
		case "trunk", "ignore":
			score -= 100
		}
	}
	return score
}

func attachmentProtoPriority(proto string) int {
	switch strings.ToLower(strings.TrimSpace(proto)) {
	case "manual":
		return 4
	case "lldp":
		return 3
	case "cdp":
		return 3
	case NeighborProtocolFDB:
		return 1
	default:
		return 2
	}
}

func attachmentCandidateBetter(pri, linkSc, sc, bestPri, bestLink, bestSc int, tieAt, bestTieAt time.Time) bool {
	if pri > bestPri {
		return true
	}
	if pri < bestPri {
		return false
	}
	if linkSc > bestLink {
		return true
	}
	if linkSc < bestLink {
		return false
	}
	if sc > bestSc {
		return true
	}
	if sc < bestSc {
		return false
	}
	if tieAt.IsZero() && bestTieAt.IsZero() {
		return false
	}
	return tieAt.After(bestTieAt)
}

// FindDeviceAttachment ищет порт свитча/роутера, к которому привязан узел (manual / LLDP / CDP / FDB).
func (s *Store) FindDeviceAttachment(ctx context.Context, deviceID int64) (*DeviceAttachment, error) {
	if deviceID <= 0 {
		return nil, nil
	}
	devices, err := s.ListDevices(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]models.Device, len(devices))
	for _, d := range devices {
		byID[d.ID] = d
	}
	target, ok := byID[deviceID]
	if !ok {
		return nil, nil
	}
	cat := NormalizeDeviceCategory(target.DeviceCategory)
	isInfra := cat == DeviceCategorySwitch || cat == DeviceCategoryRouter
	idx := buildDeviceNameIndex(devices)

	var best *DeviceAttachment
	var bestPri, bestLink, bestSc int
	var bestTieAt time.Time
	fdbCtx, _ := s.LoadFDBPortTopoContext(ctx)
	consider := func(parentID int64, ifIndex int, proto, portHint, portRole string, tieAt time.Time) {
		if parentID <= 0 || parentID == deviceID {
			return
		}
		p, ok := byID[parentID]
		if !ok {
			return
		}
		pcat := NormalizeDeviceCategory(p.DeviceCategory)
		portRole = strings.ToLower(strings.TrimSpace(portRole))
		// FDB с trunk — AP uplink; attachment score ниже access.
		if pcat != DeviceCategorySwitch && pcat != DeviceCategoryRouter && pcat != DeviceCategoryAP {
			if strings.EqualFold(proto, NeighborProtocolFDB) || strings.EqualFold(proto, "fdb") {
				return
			}
		}
		port := strings.TrimSpace(portHint)
		if port == "" && ifIndex > 0 {
			if name, err := s.GetInterfaceName(ctx, parentID, ifIndex); err == nil {
				port = name
			} else {
				port = fmt.Sprintf("%d", ifIndex)
			}
		}
		a := &DeviceAttachment{
			ParentID:       parentID,
			ParentName:     strings.TrimSpace(p.Name),
			ParentHost:     strings.TrimSpace(p.Host),
			ParentCategory: pcat,
			PortName:       port,
			IfIndex:        ifIndex,
			Protocol:       proto,
		}
		pri := attachmentProtoPriority(proto)
		linkSc := 0
		pk := portKey(parentID, ifIndex)
		resolvedRole := strings.ToLower(strings.TrimSpace(portRole))
		trunkAPOK := true
		if strings.EqualFold(proto, NeighborProtocolFDB) {
			role := resolvedRole
			if role == "" && fdbCtx != nil {
				role = strings.ToLower(strings.TrimSpace(fdbCtx.PortRole[pk]))
			}
			// ПК/камера на trunk — только VLAN-flood с core; никогда attachment.
			if !fdbTopoLinkEligible(role, cat) {
				return
			}
			if fdbCtx != nil {
				macN := fdbCtx.MACCount[pk]
				apN := fdbCtx.APDeviceCount[pk]
				invN := fdbCtx.InventoryMACCount[pk]
				sibling := fdbCtx.MaxSiblingTrunkAP[pk]
				if !fdbTopoPortEligible(role, macN, apN, invN, sibling) {
					return
				}
				linkSc = fdbAPLinkScore(role, macN, apN, invN, sibling)
				if role == "trunk" {
					trunkAPOK = linkSc >= fdbAPLinkMinTrunkScore
				}
			} else if role == "trunk" || role == "ignore" {
				return
			}
		}
		sc := attachmentScore(pcat, cat, proto, resolvedRole, trunkAPOK)
		if attachmentCandidateBetter(pri, linkSc, sc, bestPri, bestLink, bestSc, tieAt, bestTieAt) {
			bestPri, bestLink, bestSc = pri, linkSc, sc
			bestTieAt = tieAt
			best = a
		}
	}

	// 1) Ручные связи
	manuals, err := s.ListActiveManualLinks(ctx)
	if err != nil {
		return nil, err
	}
	for _, ml := range manuals {
		var parentID int64
		var ifIndex int
		switch {
		case ml.ADeviceID == deviceID:
			parentID, ifIndex = ml.BDeviceID, ml.BIfIndex
		case ml.BDeviceID == deviceID:
			parentID, ifIndex = ml.ADeviceID, ml.AIfIndex
		default:
			continue
		}
		consider(parentID, ifIndex, "manual", "", "access", time.Time{})
	}

	// 1b) Uplink свитча/роутера: LLDP/CDP на его trunk смотрит на родительский коммутатор.
	if isInfra {
		if err := s.considerLocalUplinkNeighbors(ctx, deviceID, idx, byID, consider); err != nil {
			return nil, err
		}
	}

	// 2) Соседи на портах infra-устройств → resolve remote == deviceID
	rows, err := s.pool.Query(ctx, `
		SELECT pn.device_id, pn.if_index, pn.protocol,
		       pn.remote_sys_name, pn.remote_port_id, pn.remote_chassis_id, pn.remote_mgmt_addr,
		       di.if_name, COALESCE(di.port_role, ''), di.cli_port_mode, pn.last_seen_at
		FROM port_neighbors pn
		JOIN devices d ON d.id = pn.device_id
		LEFT JOIN device_interfaces di ON di.device_id = pn.device_id AND di.if_index = pn.if_index
		WHERE pn.stale = FALSE
		  AND d.device_category IN ('switch', 'router', 'ap')
		  AND pn.last_seen_at > now() - interval '48 hours'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var parentID int64
		var ifIndex int
		var proto string
		var remSys, remPort, remChassis, remMgmt, ifName *string
		var portRole string
		var cliMode *string
		var lastSeen time.Time
		if err := rows.Scan(&parentID, &ifIndex, &proto, &remSys, &remPort, &remChassis, &remMgmt, &ifName, &portRole, &cliMode, &lastSeen); err != nil {
			return nil, err
		}
		nb := PortNeighbor{
			DeviceID:        parentID,
			IfIndex:         ifIndex,
			Protocol:        proto,
			RemoteSysName:   remSys,
			RemotePortID:    remPort,
			RemoteChassisID: remChassis,
			RemoteMgmtAddr:  remMgmt,
		}
		rid, ok := resolveRemoteDeviceID(idx, nb)
		if !ok || rid != deviceID {
			continue
		}
		portHint := ""
		if ifName != nil {
			portHint = strings.TrimSpace(*ifName)
		}
		consider(parentID, ifIndex, proto, portHint, ResolveInterfacePortRole(portRole, cliMode), lastSeen)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// 3) FDB / ARP MAC → порт на свитче (ПК без LLDP, узел только по IP).
	hexKeys, err := s.attachmentMACHexKeys(ctx, target)
	if err != nil {
		return nil, err
	}
	if len(hexKeys) > 0 {
		if err := s.considerAttachmentFromFDBMAC(ctx, hexKeys, consider); err != nil {
			return nil, err
		}
		if err := s.considerAttachmentFromFDBNeighbors(ctx, hexKeys, false, consider); err != nil {
			return nil, err
		}
		// Оффлайн: MAC уже мог пропасть из live FDB — stale port_neighbors по MAC.
		if best == nil {
			if err := s.considerAttachmentFromFDBNeighbors(ctx, hexKeys, true, consider); err != nil {
				return nil, err
			}
		}
	}

	if best != nil {
		_ = s.persistDeviceAttachment(ctx, deviceID, best)
		return best, nil
	}
	if persisted := s.loadPersistedDeviceAttachment(ctx, deviceID, byID); persisted != nil {
		return persisted, nil
	}
	return nil, nil
}

func (s *Store) persistDeviceAttachment(ctx context.Context, deviceID int64, a *DeviceAttachment) error {
	if deviceID <= 0 || a == nil || a.ParentID <= 0 || a.IfIndex <= 0 {
		return nil
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE devices SET
			attach_parent_id = $2,
			attach_if_index = $3,
			attach_protocol = $4,
			attach_updated_at = now(),
			updated_at = now()
		WHERE id = $1`, deviceID, a.ParentID, a.IfIndex, strings.TrimSpace(a.Protocol))
	return err
}

func (s *Store) loadPersistedDeviceAttachment(ctx context.Context, deviceID int64, byID map[int64]models.Device) *DeviceAttachment {
	var parentID int64
	var ifIndex int
	var proto string
	var updatedAt time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT attach_parent_id, attach_if_index, COALESCE(attach_protocol, ''), attach_updated_at
		FROM devices
		WHERE id = $1 AND attach_parent_id IS NOT NULL AND attach_if_index IS NOT NULL
		  AND attach_updated_at IS NOT NULL`, deviceID).Scan(&parentID, &ifIndex, &proto, &updatedAt)
	if err != nil {
		return nil
	}
	if time.Since(updatedAt) > persistedAttachmentMaxAge {
		return nil
	}
	p, ok := byID[parentID]
	if !ok {
		return nil
	}
	port := ""
	if name, err := s.GetInterfaceName(ctx, parentID, ifIndex); err == nil {
		port = strings.TrimSpace(name)
	}
	if port == "" {
		port = fmt.Sprintf("%d", ifIndex)
	}
	return &DeviceAttachment{
		ParentID:       parentID,
		ParentName:     strings.TrimSpace(p.Name),
		ParentHost:     strings.TrimSpace(p.Host),
		ParentCategory: NormalizeDeviceCategory(p.DeviceCategory),
		PortName:       port,
		Protocol:       strings.TrimSpace(proto),
	}
}

// deviceAttachmentMACHexKeys — MAC узла для поиска в FDB (chassis_mac + ARP по host/IP).
func deviceAttachmentMACHexKeys(chassisMAC *string, arpHex []string) []string {
	seen := make(map[string]struct{}, len(arpHex)+1)
	var out []string
	add := func(hex string) {
		hex = strings.ToLower(strings.TrimSpace(hex))
		if len(hex) != 12 {
			return
		}
		if _, dup := seen[hex]; dup {
			return
		}
		seen[hex] = struct{}{}
		out = append(out, hex)
	}
	if chassisMAC != nil {
		if mac, ok := NormalizeMACQuery(*chassisMAC); ok {
			add(macHexDigits(mac))
		}
	}
	for _, h := range arpHex {
		add(h)
	}
	return out
}

func (s *Store) attachmentMACHexKeys(ctx context.Context, d models.Device) ([]string, error) {
	arpHex, err := s.listARPMACHexForHost(ctx, d.Host)
	if err != nil {
		return nil, err
	}
	if len(arpHex) == 1 && (d.ChassisMAC == nil || strings.TrimSpace(*d.ChassisMAC) == "") {
		if mac, ok := FormatFullMAC(arpHex[0]); ok {
			_ = s.backfillDeviceChassisMAC(ctx, d.ID, mac)
		}
	}
	return deviceAttachmentMACHexKeys(d.ChassisMAC, arpHex), nil
}

// backfillDeviceChassisMAC — chassis_mac из ARP (ПК/камера без LLDP), если поле пустое.
func (s *Store) backfillDeviceChassisMAC(ctx context.Context, deviceID int64, mac string) error {
	if deviceID <= 0 {
		return nil
	}
	mac, ok := FormatFullMAC(mac)
	if !ok {
		return nil
	}
	if err := s.CheckDeviceIdentity(ctx, "", &mac, deviceID); err != nil {
		return nil
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE devices SET chassis_mac = $2, updated_at = now()
		WHERE id = $1 AND (chassis_mac IS NULL OR btrim(chassis_mac) = '')`,
		deviceID, mac)
	return err
}

func (s *Store) listARPMACHexForHost(ctx context.Context, host string) ([]string, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT lower(replace(replace(mac, ':', ''), '-', ''))
		FROM device_arp_entries
		WHERE lower(btrim(ip)) = lower(btrim($1))
		  AND length(lower(replace(replace(mac, ':', ''), '-', ''))) = 12`,
		host)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var hex string
		if err := rows.Scan(&hex); err != nil {
			return nil, err
		}
		out = append(out, hex)
	}
	return out, rows.Err()
}

func infraUplinkLocalPortEligible(proto, localPortRole string) bool {
	if strings.EqualFold(strings.TrimSpace(proto), "manual") {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(localPortRole)) {
	case "trunk", "ignore":
		return true
	default:
		return false
	}
}

func isInfraParentCategory(cat string) bool {
	switch NormalizeDeviceCategory(cat) {
	case DeviceCategorySwitch, DeviceCategoryRouter, DeviceCategoryAP:
		return true
	default:
		return false
	}
}

// considerLocalUplinkNeighbors — LLDP/CDP/manual на trunk uplink самого свитча → родительский порт.
func (s *Store) considerLocalUplinkNeighbors(
	ctx context.Context,
	deviceID int64,
	idx deviceNameIndex,
	byID map[int64]models.Device,
	consider func(parentID int64, ifIndex int, proto, portHint, portRole string, tieAt time.Time),
) error {
	rows, err := s.pool.Query(ctx, `
		SELECT pn.if_index, pn.protocol,
		       pn.remote_sys_name, pn.remote_port_id, pn.remote_chassis_id, pn.remote_mgmt_addr,
		       COALESCE(di.if_name, ''), COALESCE(di.port_role, ''), di.cli_port_mode,
		       pn.last_seen_at
		FROM port_neighbors pn
		LEFT JOIN device_interfaces di ON di.device_id = pn.device_id AND di.if_index = pn.if_index
		WHERE pn.device_id = $1
		  AND pn.stale = FALSE
		  AND pn.protocol IN ('lldp', 'cdp', 'manual')
		  AND pn.last_seen_at > now() - interval '48 hours'`,
		deviceID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var localIf int
		var proto string
		var remSys, remPort, remChassis, remMgmt, ifName string
		var portRole string
		var cliMode *string
		var lastSeen time.Time
		if err := rows.Scan(&localIf, &proto, &remSys, &remPort, &remChassis, &remMgmt, &ifName, &portRole, &cliMode, &lastSeen); err != nil {
			return err
		}
		localRole := ResolveInterfacePortRole(portRole, cliMode)
		if !infraUplinkLocalPortEligible(proto, localRole) {
			continue
		}
		nb := PortNeighbor{
			DeviceID:        deviceID,
			IfIndex:         localIf,
			Protocol:        proto,
			RemoteSysName:   attachmentStrPtr(remSys),
			RemotePortID:    attachmentStrPtr(remPort),
			RemoteChassisID: attachmentStrPtr(remChassis),
			RemoteMgmtAddr:  attachmentStrPtr(remMgmt),
		}
		parentID, ok := resolveRemoteDeviceID(idx, nb)
		if !ok || parentID == deviceID {
			continue
		}
		parent, ok := byID[parentID]
		if !ok || !isInfraParentCategory(parent.DeviceCategory) {
			continue
		}
		parentIf := 0
		portHint := strings.TrimSpace(remPort)
		if portHint != "" {
			if parsed, ok := ParseRemotePortIfIndex(portHint); ok {
				parentIf = parsed
			}
		}
		if parentIf <= 0 {
			continue
		}
		consider(parentID, parentIf, proto, portHint, "trunk", lastSeen)
	}
	return rows.Err()
}

func attachmentStrPtr(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

func (s *Store) considerAttachmentFromFDBMAC(
	ctx context.Context,
	hexKeys []string,
	consider func(parentID int64, ifIndex int, proto, portHint, portRole string, tieAt time.Time),
) error {
	rows, err := s.pool.Query(ctx, `
		SELECT f.device_id, f.if_index,
		       COALESCE(di.if_name, ''), COALESCE(di.port_role, ''), di.cli_port_mode,
		       f.first_seen_at
		FROM device_fdb_entries f
		JOIN devices d ON d.id = f.device_id
		LEFT JOIN device_interfaces di ON di.device_id = f.device_id AND di.if_index = f.if_index
		WHERE d.device_category IN ('switch', 'router', 'ap')
		  AND lower(replace(replace(f.mac, ':', ''), '-', '')) = ANY($1)
		ORDER BY f.first_seen_at DESC`,
		hexKeys)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var parentID int64
		var ifIndex int
		var ifName, portRole string
		var cliMode *string
		var firstSeen time.Time
		if err := rows.Scan(&parentID, &ifIndex, &ifName, &portRole, &cliMode, &firstSeen); err != nil {
			return err
		}
		consider(parentID, ifIndex, NeighborProtocolFDB, strings.TrimSpace(ifName), ResolveInterfacePortRole(portRole, cliMode), firstSeen)
	}
	return rows.Err()
}

func (s *Store) considerAttachmentFromFDBNeighbors(
	ctx context.Context,
	hexKeys []string,
	allowStale bool,
	consider func(parentID int64, ifIndex int, proto, portHint, portRole string, tieAt time.Time),
) error {
	staleClause := "AND pn.stale = FALSE"
	if allowStale {
		staleClause = ""
	}
	cutoff := time.Now().Add(-attachmentNeighborMaxAge)
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		SELECT pn.device_id, pn.if_index,
		       COALESCE(di.if_name, ''), COALESCE(di.port_role, ''), di.cli_port_mode,
		       pn.last_seen_at
		FROM port_neighbors pn
		JOIN devices d ON d.id = pn.device_id
		LEFT JOIN device_interfaces di ON di.device_id = pn.device_id AND di.if_index = pn.if_index
		WHERE pn.protocol = $2
		  AND d.device_category IN ('switch', 'router', 'ap')
		  AND pn.remote_chassis_id IS NOT NULL
		  AND lower(replace(replace(pn.remote_chassis_id, ':', ''), '-', '')) = ANY($1)
		  AND pn.last_seen_at > $3
		  %s
		ORDER BY pn.stale ASC, pn.last_seen_at DESC`, staleClause),
		hexKeys, NeighborProtocolFDB, cutoff)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var parentID int64
		var ifIndex int
		var ifName, portRole string
		var cliMode *string
		var lastSeen time.Time
		if err := rows.Scan(&parentID, &ifIndex, &ifName, &portRole, &cliMode, &lastSeen); err != nil {
			return err
		}
		consider(parentID, ifIndex, NeighborProtocolFDB, strings.TrimSpace(ifName), ResolveInterfacePortRole(portRole, cliMode), lastSeen)
	}
	return rows.Err()
}
