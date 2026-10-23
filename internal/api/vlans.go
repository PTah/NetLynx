package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/models"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/swcfg"
	"github.com/go-chi/chi/v5"
)

func (s *Server) deviceConfigTextForVLAN(ctx context.Context, deviceID int64) (text, via string) {
	if raw, ok := s.getDeviceRunningConfig(deviceID, CardCLISyncMaxAge); ok {
		return raw, "config_cache"
	}
	snap, err := s.st.GetLatestConfigSnapshot(ctx, deviceID)
	if err != nil || snap == nil || strings.TrimSpace(snap.ConfigText) == "" {
		return "", "ports"
	}
	return snap.ConfigText, "snapshot"
}

func ifaceName(p models.DeviceInterface) string {
	if p.IfName != nil && strings.TrimSpace(*p.IfName) != "" {
		return strings.TrimSpace(*p.IfName)
	}
	return fmt.Sprintf("ifIndex %d", p.IfIndex)
}

func skipLogicalIface(p models.DeviceInterface) bool {
	n := strings.ToLower(ifaceName(p))
	return strings.HasPrefix(n, "vlan") || strings.Contains(n, "loopback")
}

func (s *Server) deviceVLANInventory(ctx context.Context, deviceID int64) (source string, inv []swcfg.VLANInventoryRow, err error) {
	ifaces, err := s.st.ListInterfacesByDevice(ctx, deviceID)
	if err != nil {
		return "", nil, err
	}
	hints := make([]swcfg.PortVLANHint, 0, len(ifaces))
	nameByIdx := map[int]string{}
	roleByIdx := map[int]string{}
	for _, p := range ifaces {
		if p.IfIndex <= 0 || skipLogicalIface(p) {
			continue
		}
		h := swcfg.PortVLANHint{
			IfIndex: p.IfIndex,
			IfName:  ifaceName(p),
			Role:    store.ResolveInterfacePortRole(p.PortRole, p.CLIPortMode),
		}
		if p.CliAccessVlan != nil && *p.CliAccessVlan > 0 {
			h.AccessVLAN = p.CliAccessVlan
		}
		if p.VlanID != nil && *p.VlanID > 0 {
			h.FDBVLAN = p.VlanID
		}
		hints = append(hints, h)
		nameByIdx[p.IfIndex] = h.IfName
		roleByIdx[p.IfIndex] = h.Role
	}
	fdbRows, _ := s.st.ListFDBVLANPorts(ctx, deviceID)
	fdb := make([]swcfg.FDBVLANPort, 0, len(fdbRows))
	for _, e := range fdbRows {
		fdb = append(fdb, swcfg.FDBVLANPort{
			IfIndex: e.IfIndex,
			VLANID:  e.VLANID,
			IfName:  nameByIdx[e.IfIndex],
			Role:    roleByIdx[e.IfIndex],
		})
	}
	cfg, via := s.deviceConfigTextForVLAN(ctx, deviceID)
	inv = swcfg.BuildVLANInventory(cfg, hints, fdb)
	if inv == nil {
		inv = []swcfg.VLANInventoryRow{}
	}
	return via, inv, nil
}

func (s *Server) handleGetDeviceVLANs(w http.ResponseWriter, r *http.Request) {
	deviceID, err := parseDeviceID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "неверный id")
		return
	}
	via, inv, err := s.deviceVLANInventory(r.Context(), deviceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"source": via,
		"vlans":  inv,
	})
}

// vlanIDsForPortChange — VLAN, которые должны уже быть в vlan database перед apply.
func vlanIDsForPortChange(ch swcfg.PortVLANChange) []int {
	switch ch.Op {
	case swcfg.VLANOpSetAccess, swcfg.VLANOpAddTagged, swcfg.VLANOpSetNative:
		if ch.VLANID > 0 {
			return []int{ch.VLANID}
		}
	case swcfg.VLANOpTrunkAllow:
		var ids []int
		if ch.VLANID > 0 {
			ids = append(ids, ch.VLANID)
		}
		mode := strings.ToLower(strings.TrimSpace(ch.AllowedMode))
		if mode == swcfg.TrunkAllowAdd || mode == swcfg.TrunkAllowExcept || mode == swcfg.TrunkAllowSet {
			ids = append(ids, swcfg.ParseVLANIDList(swcfg.NormalizeVLANList(ch.AllowedList))...)
		}
		return ids
	}
	return nil
}

// requireVLANsInDatabase — если в show run есть vlan database, запрещает ссылаться на отсутствующие VLAN.
// Без конфига / пустой database проверку пропускаем (не блокируем офлайн).
func (s *Server) requireVLANsInDatabase(ctx context.Context, deviceID int64, ids ...int) error {
	if len(ids) == 0 {
		return nil
	}
	cfg, _ := s.deviceConfigTextForVLAN(ctx, deviceID)
	if strings.TrimSpace(cfg) == "" {
		return nil
	}
	db := swcfg.ParseVLANDatabase(cfg)
	if len(db) == 0 {
		return nil
	}
	for _, id := range ids {
		if id == 1 {
			continue
		}
		if id < 1 || id > 4094 {
			return fmt.Errorf("vlan_id должен быть 1–4094")
		}
		if _, ok := db[id]; !ok {
			return fmt.Errorf("VLAN %d нет в vlan database свитча — сначала создайте его на вкладке VLAN", id)
		}
	}
	return nil
}

type patchPortVLANBody struct {
	Op          string `json:"op"`
	VLANID      int    `json:"vlan_id"`
	AllowedMode string `json:"allowed_mode"`
	AllowedList string `json:"allowed_vlans"`
}

func (s *Server) handlePatchPortVLAN(w http.ResponseWriter, r *http.Request) {
	if !s.requireOperatorOrAdmin(w, r) {
		return
	}
	deviceID, ifIndex, ok := parseDeviceIfIndex(w, r)
	if !ok {
		return
	}
	var body patchPortVLANBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON")
		return
	}
	ch := swcfg.PortVLANChange{
		Op:          strings.TrimSpace(body.Op),
		VLANID:      body.VLANID,
		AllowedMode: strings.TrimSpace(body.AllowedMode),
		AllowedList: body.AllowedList,
	}
	ifaces, err := s.st.ListInterfacesByDevice(r.Context(), deviceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var found *models.DeviceInterface
	for i := range ifaces {
		if ifaces[i].IfIndex == ifIndex {
			found = &ifaces[i]
			break
		}
	}
	if found == nil {
		writeError(w, http.StatusNotFound, "порт не найден")
		return
	}
	ch.PortMode = store.ResolveInterfacePortRole(found.PortRole, found.CLIPortMode)
	if found.CliAccessVlan != nil {
		ch.PrevAccessVLAN = found.CliAccessVlan
	}
	if err := ch.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.requireVLANsInDatabase(r.Context(), deviceID, vlanIDsForPortChange(ch)...); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.guardMikrotikRouterPortWrite(r.Context(), deviceID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	creds, iface, err := s.resolvePortSSH(r.Context(), deviceID, ifIndex)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if err := swcfg.ApplyPortChange(creds, swcfg.PortChange{Interface: iface, VLAN: &ch, Write: true}); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	_ = s.st.UpdateInterfaceVLANAfterCLI(r.Context(), deviceID, ifIndex, ch.Op, ch.VLANID)
	// Ответ — сразу после CLI. show run на SNR часто занимает десятки секунд и больше;
	// раньше UI молчал всё это время, хотя VLAN на порту уже был записан.
	s.invalidateDeviceRunningConfig(deviceID)
	s.refreshConfigAfterVLANDBAsync(deviceID, creds)
	via, inv, _ := s.deviceVLANInventory(r.Context(), deviceID)
	inv = swcfg.OverlayPortVLAN(inv, ifIndex, iface, ch)
	msg := portVLANAppliedMessage(iface, ch)
	details := map[string]interface{}{
		"if_index":      ifIndex,
		"op":            ch.Op,
		"vlan_id":       ch.VLANID,
		"allowed_mode":  ch.AllowedMode,
		"allowed_vlans": swcfg.NormalizeVLANList(ch.AllowedList),
		"via":           "ssh",
		"message":       msg,
	}
	s.audit(r, "port.vlan.update", "device", &deviceID, details)
	s.emitConfigEditEvent(r, deviceID, &ifIndex, "port.vlan", details)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok": true, "op": ch.Op, "vlan_id": ch.VLANID, "via": "ssh",
		"allowed_mode": ch.AllowedMode, "allowed_vlans": swcfg.NormalizeVLANList(ch.AllowedList),
		"message": msg,
		"source":  via, "vlans": inv,
	})
}

type patchDeviceVLANBody struct {
	Name *string `json:"name"`
}

type createDeviceVLANBody struct {
	VLANID int     `json:"vlan_id"`
	Name   *string `json:"name"`
}

func parseVLANIDParam(r *http.Request) (int, error) {
	raw := strings.TrimSpace(chi.URLParam(r, "vlanId"))
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > 4094 {
		return 0, fmt.Errorf("неверный vlan_id")
	}
	return n, nil
}

func (s *Server) refreshConfigAfterVLANDBAsync(deviceID int64, creds swcfg.Creds) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		s.refreshConfigAfterVLANDB(ctx, deviceID, creds)
	}()
}

func (s *Server) refreshConfigAfterVLANDB(ctx context.Context, deviceID int64, creds swcfg.Creds) {
	s.invalidateDeviceRunningConfig(deviceID)
	var raw []byte
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				slog.Warn("vlan config refresh cancelled", "device_id", deviceID, "err", ctx.Err())
				return
			case <-time.After(time.Duration(attempt) * 700 * time.Millisecond):
			}
		}
		raw, err = swcfg.FetchConfig(creds)
		if err == nil && len(raw) > 0 {
			break
		}
	}
	if err != nil || len(raw) == 0 {
		slog.Warn("vlan config refresh failed", "device_id", deviceID, "err", err)
		return
	}
	s.putDeviceRunningConfig(deviceID, raw)
	_, _, _ = s.st.SaveConfigSnapshotIfChanged(ctx, deviceID, string(raw), "port_sync")
}

func portVLANAppliedMessage(ifName string, ch swcfg.PortVLANChange) string {
	port := strings.TrimSpace(ifName)
	if port == "" {
		port = "порту"
	}
	tail := " Команда на коммутаторе выполнена; список может ещё раз обновиться после show run."
	switch ch.Op {
	case swcfg.VLANOpNoVLAN:
		return "Готово: с порта " + port + " снят VLAN (No VLAN)." + tail
	case swcfg.VLANOpSetAccess:
		return fmt.Sprintf("Готово: на порт %s записан VLAN %d (access).%s", port, ch.VLANID, tail)
	case swcfg.VLANOpRemove:
		return fmt.Sprintf("Готово: VLAN %d убран с порта %s.%s", ch.VLANID, port, tail)
	case swcfg.VLANOpSetNative:
		return fmt.Sprintf("Готово: на порту %s native VLAN %d.%s", port, ch.VLANID, tail)
	case swcfg.VLANOpAddTagged:
		return fmt.Sprintf("Готово: VLAN %d добавлен как tagged на порт %s.%s", ch.VLANID, port, tail)
	case swcfg.VLANOpTrunkAllow:
		mode := strings.ToLower(strings.TrimSpace(ch.AllowedMode))
		list := swcfg.NormalizeVLANList(ch.AllowedList)
		detail := mode
		if mode != swcfg.TrunkAllowAll && list != "" {
			detail = mode + " " + list
		}
		if ch.VLANID > 0 {
			detail += fmt.Sprintf(", native %d", ch.VLANID)
		}
		return fmt.Sprintf("Готово: на порту %s записан trunk allowed vlan (%s).%s", port, detail, tail)
	default:
		return "Готово: VLAN на порту " + port + " записан." + tail
	}
}

func vlanDBAppliedMessage(ch swcfg.VLANDatabaseChange) string {
	tail := " Команда на коммутаторе выполнена; список может ещё раз обновиться после show run."
	switch ch.Op {
	case swcfg.VLANDBOpCreate:
		name := strings.TrimSpace(ch.Name)
		if name != "" {
			return fmt.Sprintf("Готово: VLAN %d (%s) создан в vlan database.%s", ch.VLANID, name, tail)
		}
		return fmt.Sprintf("Готово: VLAN %d создан в vlan database.%s", ch.VLANID, tail)
	case swcfg.VLANDBOpSetName:
		return fmt.Sprintf("Готово: имя VLAN %d записано (%s).%s", ch.VLANID, strings.TrimSpace(ch.Name), tail)
	case swcfg.VLANDBOpDelete:
		return fmt.Sprintf("Готово: VLAN %s удалён из vlan database.%s", swcfg.FormatVLANIDList(ch.DeleteIDs()), tail)
	default:
		return "Готово: vlan database обновлена." + tail
	}
}

func (s *Server) handleCreateDeviceVLAN(w http.ResponseWriter, r *http.Request) {
	if !s.requireOperatorOrAdmin(w, r) {
		return
	}
	deviceID, err := parseDeviceID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "неверный id")
		return
	}
	var body createDeviceVLANBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON")
		return
	}
	name := ""
	if body.Name != nil {
		name = *body.Name
	}
	ch := swcfg.VLANDatabaseChange{Op: swcfg.VLANDBOpCreate, VLANID: body.VLANID, Name: name}
	if err := ch.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.guardMikrotikRouterPortWrite(r.Context(), deviceID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	creds, err := s.resolveDeviceSSH(r.Context(), deviceID)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if err := swcfg.ApplyVLANDatabaseChange(creds, ch); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	s.invalidateDeviceRunningConfig(deviceID)
	s.refreshConfigAfterVLANDBAsync(deviceID, creds)
	via, inv, _ := s.deviceVLANInventory(r.Context(), deviceID)
	inv = swcfg.OverlayVLANDatabase(inv, ch)
	msg := vlanDBAppliedMessage(ch)
	details := map[string]interface{}{
		"vlan_id": body.VLANID, "name": name, "via": "ssh", "message": msg,
	}
	s.audit(r, "vlan.database.create", "device", &deviceID, details)
	s.emitConfigEditEvent(r, deviceID, nil, "vlan.database.create", details)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok": true, "vlan_id": body.VLANID, "name": name, "via": "ssh",
		"message": msg,
		"source":  via, "vlans": inv,
	})
}

func (s *Server) handlePatchDeviceVLAN(w http.ResponseWriter, r *http.Request) {
	if !s.requireOperatorOrAdmin(w, r) {
		return
	}
	deviceID, err := parseDeviceID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "неверный id")
		return
	}
	vlanID, err := parseVLANIDParam(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var body patchDeviceVLANBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON")
		return
	}
	name := ""
	if body.Name != nil {
		name = *body.Name
	}
	ch := swcfg.VLANDatabaseChange{Op: swcfg.VLANDBOpSetName, VLANID: vlanID, Name: name}
	if err := ch.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.guardMikrotikRouterPortWrite(r.Context(), deviceID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	creds, err := s.resolveDeviceSSH(r.Context(), deviceID)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if err := swcfg.ApplyVLANDatabaseChange(creds, ch); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	s.invalidateDeviceRunningConfig(deviceID)
	s.refreshConfigAfterVLANDBAsync(deviceID, creds)
	via, inv, _ := s.deviceVLANInventory(r.Context(), deviceID)
	inv = swcfg.OverlayVLANDatabase(inv, ch)
	msg := vlanDBAppliedMessage(ch)
	details := map[string]interface{}{
		"vlan_id": vlanID, "name": name, "via": "ssh", "message": msg,
	}
	s.audit(r, "vlan.database.name", "device", &deviceID, details)
	s.emitConfigEditEvent(r, deviceID, nil, "vlan.database.name", details)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok": true, "vlan_id": vlanID, "name": name, "via": "ssh",
		"message": msg,
		"source":  via, "vlans": inv,
	})
}

func (s *Server) handleDeleteDeviceVLAN(w http.ResponseWriter, r *http.Request) {
	if !s.requireOperatorOrAdmin(w, r) {
		return
	}
	deviceID, err := parseDeviceID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "неверный id")
		return
	}
	vlanID, err := parseVLANIDParam(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.applyDeleteDeviceVLANs(w, r, deviceID, []int{vlanID})
}

type deleteDeviceVLANsBody struct {
	VLANIDs []int `json:"vlan_ids"`
}

func (s *Server) handleDeleteDeviceVLANs(w http.ResponseWriter, r *http.Request) {
	if !s.requireOperatorOrAdmin(w, r) {
		return
	}
	deviceID, err := parseDeviceID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "неверный id")
		return
	}
	var body deleteDeviceVLANsBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON")
		return
	}
	s.applyDeleteDeviceVLANs(w, r, deviceID, body.VLANIDs)
}

func (s *Server) applyDeleteDeviceVLANs(w http.ResponseWriter, r *http.Request, deviceID int64, ids []int) {
	ch := swcfg.VLANDatabaseChange{Op: swcfg.VLANDBOpDelete, VLANIDs: ids}
	if len(ids) == 1 {
		ch.VLANID = ids[0]
	}
	if err := ch.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.guardMikrotikRouterPortWrite(r.Context(), deviceID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	_, inv, invErr := s.deviceVLANInventory(r.Context(), deviceID)
	if invErr != nil {
		writeError(w, http.StatusInternalServerError, invErr.Error())
		return
	}
	byID := map[int]swcfg.VLANInventoryRow{}
	for _, row := range inv {
		byID[row.VLANID] = row
	}
	var blocked []string
	for _, id := range ch.DeleteIDs() {
		row, ok := byID[id]
		if !ok || !swcfg.VLANConfiguredOnPorts(row) {
			continue
		}
		blocked = append(blocked, fmt.Sprintf("VLAN %d на портах: %s", id, swcfg.FormatVLANPortBindings(row)))
	}
	if len(blocked) > 0 {
		writeError(w, http.StatusConflict, "сначала снимите VLAN с портов («Убрать с порта» / Access→другой VLAN), затем удаляйте из vlan database. "+strings.Join(blocked, "; "))
		return
	}
	// Hard-block: management SVI = devices.host
	cfgText, _ := s.deviceConfigTextForVLAN(r.Context(), deviceID)
	dev, _ := s.st.GetDevice(r.Context(), deviceID)
	host := ""
	if dev != nil {
		host = dev.Host
	}
	if vid, sip, ok := swcfg.MgmtVLANFromSVI(host, swcfg.ParseSVIAddresses(cfgText)); ok {
		for _, id := range ch.DeleteIDs() {
			if id == vid {
				writeError(w, http.StatusConflict, fmt.Sprintf(
					"удаление запрещено: VLAN %d — management SVI этого свитча (host %s = %s)",
					vid, host, sip))
				return
			}
		}
	}
	creds, err := s.resolveDeviceSSH(r.Context(), deviceID)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if err := swcfg.ApplyVLANDatabaseChange(creds, ch); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	s.invalidateDeviceRunningConfig(deviceID)
	s.refreshConfigAfterVLANDBAsync(deviceID, creds)
	via, inv, _ := s.deviceVLANInventory(r.Context(), deviceID)
	inv = swcfg.OverlayVLANDatabase(inv, ch)
	deleted := ch.DeleteIDs()
	msg := vlanDBAppliedMessage(ch)
	details := map[string]interface{}{
		"vlan_ids": deleted, "via": "ssh", "message": msg,
	}
	s.audit(r, "vlan.database.delete", "device", &deviceID, details)
	s.emitConfigEditEvent(r, deviceID, nil, "vlan.database.delete", details)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok": true, "vlan_ids": deleted, "via": "ssh",
		"message": msg,
		"source":  via, "vlans": inv,
	})
}
