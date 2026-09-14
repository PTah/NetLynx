package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/models"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/swcfg"
	"github.com/go-chi/chi/v5"
)

type postPortPoEResetBody struct {
	Seconds int `json:"seconds"`
}

func (s *Server) handlePostPortPoEReset(w http.ResponseWriter, r *http.Request) {
	if !s.requireOperatorOrAdmin(w, r) {
		return
	}
	deviceID, ifIndex, ok := parseDeviceIfIndex(w, r)
	if !ok {
		return
	}
	var body postPortPoEResetBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON")
		return
	}
	sec, err := swcfg.NormalizePoEResetSeconds(body.Seconds)
	if err != nil {
		// По умолчанию 10 с, как в типовом EdgeSwitch `#poe reset 10` / interface poe reset 10.
		if body.Seconds == 0 {
			sec = 10
		} else {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	via, err := s.pushPortPoEReset(r.Context(), deviceID, []int{ifIndex}, sec)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	details := map[string]interface{}{
		"if_index": ifIndex,
		"seconds":  sec,
		"via":      via,
	}
	s.audit(r, "port.poe.reset", "device", &deviceID, details)
	s.emitConfigEditEvent(r, deviceID, &ifIndex, "port.poe_reset", details)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok": true, "seconds": sec, "via": via,
	})
}

type bulkPortVLANBody struct {
	Op     string `json:"op"`
	VLANID int    `json:"vlan_id"`
}

type postBulkPortsBody struct {
	IfIndexes        []int              `json:"if_indexes"`
	AdminUp          *bool              `json:"admin_up"`
	PoEMode          *string            `json:"poe_mode"`
	VLAN             *bulkPortVLANBody  `json:"vlan"`
	PoEResetSeconds  *int               `json:"poe_reset_seconds"`
}

func (s *Server) handlePostBulkPorts(w http.ResponseWriter, r *http.Request) {
	if !s.requireOperatorOrAdmin(w, r) {
		return
	}
	deviceID, err := parseDeviceID(chi.URLParam(r, "id"))
	if err != nil || deviceID <= 0 {
		writeError(w, http.StatusBadRequest, "неверный device id")
		return
	}
	var body postBulkPortsBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON")
		return
	}
	idxs := uniquePositiveInts(body.IfIndexes)
	if len(idxs) < 2 {
		writeError(w, http.StatusBadRequest, "для массовой настройки нужно минимум 2 порта (if_indexes)")
		return
	}
	hasAdmin := body.AdminUp != nil
	hasPoE := body.PoEMode != nil && strings.TrimSpace(*body.PoEMode) != ""
	hasVLAN := body.VLAN != nil && strings.TrimSpace(body.VLAN.Op) != ""
	hasReset := body.PoEResetSeconds != nil
	if !hasAdmin && !hasPoE && !hasVLAN && !hasReset {
		writeError(w, http.StatusBadRequest, "нечего менять: укажите admin_up, poe_mode, vlan или poe_reset_seconds")
		return
	}
	if err := s.guardMikrotikRouterPortWrite(r.Context(), deviceID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	applied := map[string]interface{}{}
	notes := []string{}

	if hasAdmin {
		status := 2
		if *body.AdminUp {
			status = 1
		}
		via, err := s.pushBulkPortAdmin(r.Context(), deviceID, idxs, status)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		for _, ifIndex := range idxs {
			_ = s.st.UpdateInterfaceAdminStatus(r.Context(), deviceID, ifIndex, status)
		}
		applied["admin_up"] = *body.AdminUp
		applied["admin_via"] = via
		if *body.AdminUp {
			notes = append(notes, "admin вкл")
		} else {
			notes = append(notes, "admin выкл (shutdown)")
		}
	}

	if hasPoE {
		mode, err := swcfg.NormalizePoEMode(*body.PoEMode)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		via, err := s.pushBulkPortPoE(r.Context(), deviceID, idxs, mode)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		applied["poe_mode"] = mode
		applied["poe_via"] = via
		notes = append(notes, "PoE "+mode)
	}

	if hasVLAN {
		op := strings.TrimSpace(body.VLAN.Op)
		if op != "set_access" && op != "no_vlan" {
			writeError(w, http.StatusBadRequest, "vlan.op: set_access или no_vlan")
			return
		}
		via, err := s.pushBulkPortVLAN(r.Context(), deviceID, idxs, op, body.VLAN.VLANID)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		applied["vlan_op"] = op
		applied["vlan_id"] = body.VLAN.VLANID
		applied["vlan_via"] = via
		if op == "no_vlan" {
			notes = append(notes, "VLAN: No VLAN")
		} else {
			notes = append(notes, fmt.Sprintf("VLAN access %d", body.VLAN.VLANID))
		}
	}

	if hasReset {
		sec, err := swcfg.NormalizePoEResetSeconds(*body.PoEResetSeconds)
		if err != nil {
			if *body.PoEResetSeconds == 0 {
				sec = 10
			} else {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
		}
		via, err := s.pushPortPoEReset(r.Context(), deviceID, idxs, sec)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		applied["poe_reset_seconds"] = sec
		applied["poe_reset_via"] = via
		notes = append(notes, fmt.Sprintf("PoE reset %ds", sec))
	}

	details := map[string]interface{}{
		"if_indexes": idxs,
		"applied":    applied,
		"notes":      notes,
		"summary":    formatBulkPortSummary(idxs, notes),
	}
	s.audit(r, "port.bulk.update", "device", &deviceID, details)
	// if_index в events — первый порт; полный список в if_indexes / summary.
	first := idxs[0]
	s.emitConfigEditEvent(r, deviceID, &first, "port.bulk", details)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok": true, "if_indexes": idxs, "applied": applied,
	})
}

func formatBulkPortSummary(idxs []int, notes []string) string {
	ports := make([]string, 0, len(idxs))
	for _, i := range idxs {
		ports = append(ports, strconv.Itoa(i))
	}
	head := fmt.Sprintf("%d порт(ов): if %s", len(idxs), strings.Join(ports, ","))
	if len(notes) == 0 {
		return head
	}
	return head + " — " + strings.Join(notes, "; ")
}

func uniquePositiveInts(in []int) []int {
	seen := map[int]struct{}{}
	var out []int
	for _, n := range in {
		if n <= 0 {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	sort.Ints(out)
	return out
}

func (s *Server) resolvePortNames(ctx context.Context, deviceID int64, ifIndexes []int) (swcfg.Creds, []string, error) {
	creds, err := s.resolveDeviceSSH(ctx, deviceID)
	if err != nil {
		return swcfg.Creds{}, nil, err
	}
	var names []string
	for _, idx := range ifIndexes {
		name, err := s.st.GetInterfaceName(ctx, deviceID, idx)
		if err != nil {
			return swcfg.Creds{}, nil, fmt.Errorf("ifIndex %d: %w", idx, err)
		}
		names = append(names, name)
	}
	return creds, names, nil
}

// applyPortChangePreferRange — одна SSH-сессия на диапазон EdgeSwitch (0/4-0/7) или по портам.
func (s *Server) applyPortChangePreferRange(ctx context.Context, deviceID int64, ifIndexes []int, build func(iface string) swcfg.PortChange) (via string, err error) {
	if err := s.guardMikrotikRouterPortWrite(ctx, deviceID); err != nil {
		return "", err
	}
	creds, names, err := s.resolvePortNames(ctx, deviceID, ifIndexes)
	if err != nil {
		return "", err
	}
	v := swcfg.DetectVendor(string(creds.Vendor), creds.SysDescr, creds.Name)
	if rangeSpec, ok := swcfg.CompactUbiquitiInterfaceRange(names); ok && (v == swcfg.VendorUbiquiti || v == swcfg.VendorAuto) {
		ch := build(rangeSpec)
		ch.Write = true
		if err := swcfg.ApplyPortChange(creds, ch); err != nil {
			return "", err
		}
		s.invalidateDeviceRunningConfig(deviceID)
		return "ssh_range:" + rangeSpec, nil
	}
	for i, name := range names {
		ch := build(name)
		ch.Write = i == len(names)-1 // write memory только в конце
		if err := swcfg.ApplyPortChange(creds, ch); err != nil {
			return "", fmt.Errorf("%s: %w", name, err)
		}
	}
	s.invalidateDeviceRunningConfig(deviceID)
	return "ssh", nil
}

func (s *Server) pushBulkPortAdmin(ctx context.Context, deviceID int64, ifIndexes []int, status int) (string, error) {
	up := status == 1
	return s.applyPortChangePreferRange(ctx, deviceID, ifIndexes, func(iface string) swcfg.PortChange {
		return swcfg.PortChange{Interface: iface, AdminUp: &up}
	})
}

func (s *Server) pushBulkPortPoE(ctx context.Context, deviceID int64, ifIndexes []int, mode string) (string, error) {
	m := mode
	return s.applyPortChangePreferRange(ctx, deviceID, ifIndexes, func(iface string) swcfg.PortChange {
		return swcfg.PortChange{Interface: iface, PoEMode: &m}
	})
}

func (s *Server) pushBulkPortVLAN(ctx context.Context, deviceID int64, ifIndexes []int, op string, vlanID int) (string, error) {
	ifaces, err := s.st.ListInterfacesByDevice(ctx, deviceID)
	if err != nil {
		return "", err
	}
	byIdx := map[int]models.DeviceInterface{}
	for _, p := range ifaces {
		byIdx[p.IfIndex] = p
	}
	for _, idx := range ifIndexes {
		found, ok := byIdx[idx]
		if !ok {
			return "", fmt.Errorf("порт ifIndex=%d не найден", idx)
		}
		ch := swcfg.PortVLANChange{
			Op:       op,
			VLANID:   vlanID,
			PortMode: store.ResolveInterfacePortRole(found.PortRole, found.CLIPortMode),
		}
		if found.CliAccessVlan != nil {
			ch.PrevAccessVLAN = found.CliAccessVlan
		}
		if err := ch.Validate(); err != nil {
			return "", fmt.Errorf("ifIndex %d: %w", idx, err)
		}
		if err := s.requireVLANsInDatabase(ctx, deviceID, vlanIDsForPortChange(ch)...); err != nil {
			return "", err
		}
	}
	via, err := s.applyPortChangePreferRange(ctx, deviceID, ifIndexes, func(iface string) swcfg.PortChange {
		// Для range/однотипных портов берём VLAN-смену по первому порту (op/vlan_id одинаковы).
		first := byIdx[ifIndexes[0]]
		ch := swcfg.PortVLANChange{
			Op:       op,
			VLANID:   vlanID,
			PortMode: store.ResolveInterfacePortRole(first.PortRole, first.CLIPortMode),
		}
		if first.CliAccessVlan != nil {
			ch.PrevAccessVLAN = first.CliAccessVlan
		}
		return swcfg.PortChange{Interface: iface, VLAN: &ch}
	})
	if err != nil {
		return "", err
	}
	for _, idx := range ifIndexes {
		_ = s.st.UpdateInterfaceVLANAfterCLI(ctx, deviceID, idx, op, vlanID)
	}
	creds, _, rerr := s.resolvePortNames(ctx, deviceID, ifIndexes[:1])
	if rerr == nil {
		s.refreshConfigAfterVLANDB(ctx, deviceID, creds)
	}
	return via, nil
}

func (s *Server) pushPortPoEReset(ctx context.Context, deviceID int64, ifIndexes []int, seconds int) (string, error) {
	if err := s.guardMikrotikRouterPortWrite(ctx, deviceID); err != nil {
		return "", err
	}
	creds, names, err := s.resolvePortNames(ctx, deviceID, ifIndexes)
	if err != nil {
		return "", err
	}
	v := swcfg.DetectVendor(string(creds.Vendor), creds.SysDescr, creds.Name)
	plan := swcfg.PoEResetPlan(v)

	switch plan {
	case swcfg.PoEResetNative:
		sec := seconds
		via, err := s.applyPortChangePreferRange(ctx, deviceID, ifIndexes, func(iface string) swcfg.PortChange {
			return swcfg.PortChange{Interface: iface, PoEResetSeconds: &sec}
		})
		return via, err
	case swcfg.PoEResetMikrotik:
		sec := seconds
		for i, name := range names {
			ch := swcfg.PortChange{Interface: name, PoEResetSeconds: &sec, Write: i == len(names)-1}
			if err := swcfg.ApplyPortChange(creds, ch); err != nil {
				return "", fmt.Errorf("%s: %w", name, err)
			}
		}
		s.invalidateDeviceRunningConfig(deviceID)
		return "ssh_mikrotik_power_cycle", nil
	default:
		// Power-cycle: PoE off → пауза → PoE on (poe+). Не сохраняем «off» в startup надолго.
		off := "off"
		on := "poe+"
		for _, name := range names {
			if err := swcfg.ApplyPortChange(creds, swcfg.PortChange{Interface: name, PoEMode: &off, Write: false}); err != nil {
				return "", fmt.Errorf("%s off: %w", name, err)
			}
		}
		wait := time.Duration(seconds) * time.Second
		if wait < time.Second {
			wait = 10 * time.Second
		}
		time.Sleep(wait)
		for i, name := range names {
			write := i == len(names)-1
			if err := swcfg.ApplyPortChange(creds, swcfg.PortChange{Interface: name, PoEMode: &on, Write: write}); err != nil {
				return "", fmt.Errorf("%s on: %w", name, err)
			}
		}
		s.invalidateDeviceRunningConfig(deviceID)
		return "ssh_power_cycle", nil
	}
}
