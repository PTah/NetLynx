package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/backup"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/models"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/portrolesync"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/snmp"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/swcfg"
	"github.com/jackc/pgx/v5"
)

func (s *Server) requireOperatorOrAdmin(w http.ResponseWriter, r *http.Request) bool {
	if s.cfg.AuthDisabled {
		return true
	}
	role, ok := s.resolveAuthRole(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "требуется вход")
		return false
	}
	if role != roleOperator && role != roleAdmin {
		writeError(w, http.StatusForbidden, "недостаточно прав")
		return false
	}
	return true
}

func (s *Server) handlePatchPortDescr(w http.ResponseWriter, r *http.Request) {
	if !s.requireOperatorOrAdmin(w, r) {
		return
	}
	deviceID, ifIndex, ok := parseDeviceIfIndex(w, r)
	if !ok {
		return
	}
	var body patchPortDescrBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON")
		return
	}
	raw := ""
	if body.Descr != nil {
		raw = *body.Descr
	}
	descrPtr := store.NormalizeDescrOverride(raw)
	descr := ""
	if descrPtr != nil {
		descr = *descrPtr
	}

	// EdgeSwitch XP: ifAlias в SNMP нет, Fastpath CLI нет — только подпись в NetLynx.
	if dev, err := s.st.GetDevice(r.Context(), deviceID); err == nil && dev != nil {
		sys := ""
		if dev.SysDescr != nil {
			sys = *dev.SysDescr
		}
		if swcfg.IsEdgeSwitchXP(sys, dev.Name) {
			if err := s.st.UpdateInterfaceDescrOverride(r.Context(), deviceID, ifIndex, descrPtr); err != nil {
				if err == store.ErrDeviceNotFound {
					writeError(w, http.StatusNotFound, "порт не найден")
					return
				}
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			s.audit(r, "port.descr.update", "device", &deviceID, map[string]interface{}{
				"if_index": ifIndex,
				"descr":    descr,
				"via":      "local",
			})
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"ok":             true,
				"descr":          descr,
				"descr_override": descrPtr,
				"via":            "local",
			})
			return
		}
	}

	via, err := s.pushPortDescription(r.Context(), deviceID, ifIndex, descr)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if err := s.st.SyncInterfaceDescrAfterDeviceWrite(r.Context(), deviceID, ifIndex, descr); err != nil {
		if err == store.ErrDeviceNotFound {
			writeError(w, http.StatusNotFound, "порт не найден")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "port.descr.update", "device", &deviceID, map[string]interface{}{
		"if_index": ifIndex,
		"descr":    descr,
		"via":      via,
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok": true, "descr": descr, "descr_override": nil, "via": via,
	})
}

type patchPortAdminBody struct {
	AdminStatus *int  `json:"admin_status"` // 1=up, 2=down
	AdminUp     *bool `json:"admin_up"`     // альтернатива
}

func (s *Server) handlePatchPortAdmin(w http.ResponseWriter, r *http.Request) {
	if !s.requireOperatorOrAdmin(w, r) {
		return
	}
	deviceID, ifIndex, ok := parseDeviceIfIndex(w, r)
	if !ok {
		return
	}
	var body patchPortAdminBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON")
		return
	}
	status := 0
	if body.AdminStatus != nil {
		status = *body.AdminStatus
	} else if body.AdminUp != nil {
		if *body.AdminUp {
			status = 1
		} else {
			status = 2
		}
	}
	if status != 1 && status != 2 {
		writeError(w, http.StatusBadRequest, "укажите admin_status: 1 (вкл) или 2 (выкл)")
		return
	}

	via, err := s.pushPortAdmin(r.Context(), deviceID, ifIndex, status)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if err := s.st.UpdateInterfaceAdminStatus(r.Context(), deviceID, ifIndex, status); err != nil {
		if err == store.ErrDeviceNotFound {
			writeError(w, http.StatusNotFound, "порт не найден")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	action := "no_shutdown"
	if status == 2 {
		action = "shutdown"
	}
	s.audit(r, "port.admin.update", "device", &deviceID, map[string]interface{}{
		"if_index": ifIndex, "admin_status": status, "action": action, "via": via,
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok": true, "admin_status": status, "via": via,
	})
}

func (s *Server) pushPortDescription(ctx context.Context, deviceID int64, ifIndex int, descr string) (via string, err error) {
	pd, err := s.st.GetPollDevice(ctx, deviceID)
	if err != nil {
		return "", fmt.Errorf("устройство: %w", err)
	}
	if pd == nil {
		return "", store.ErrDeviceNotFound
	}
	snmpErr := s.snmpSetAlias(pd, ifIndex, descr)
	if snmpErr == nil {
		s.invalidateDeviceRunningConfig(deviceID)
		return "snmp", nil
	}
	sshErr := s.sshPortChange(ctx, deviceID, ifIndex, &descr, nil)
	if sshErr == nil {
		return "ssh", nil
	}
	return "", fmt.Errorf("не удалось записать описание на свитч (SNMP: %v; SSH: %v)", snmpErr, sshErr)
}

func (s *Server) pushPortAdmin(ctx context.Context, deviceID int64, ifIndex, status int) (via string, err error) {
	pd, err := s.st.GetPollDevice(ctx, deviceID)
	if err != nil {
		return "", fmt.Errorf("устройство: %w", err)
	}
	if pd == nil {
		return "", store.ErrDeviceNotFound
	}
	up := status == 1
	snmpErr := s.snmpSetAdmin(pd, ifIndex, status)
	if snmpErr == nil {
		return "snmp", nil
	}
	sshErr := s.sshPortChange(ctx, deviceID, ifIndex, nil, &up)
	if sshErr == nil {
		return "ssh", nil
	}
	return "", fmt.Errorf("не удалось сменить admin status (SNMP: %v; SSH: %v)", snmpErr, sshErr)
}

func (s *Server) snmpSetAlias(pd *store.PollDevice, ifIndex int, descr string) error {
	g, err := snmp.NewGoSNMP(*pd)
	if err != nil {
		return err
	}
	if err := g.Connect(); err != nil {
		return err
	}
	defer g.Conn.Close()
	return snmp.SetIfAlias(g, ifIndex, descr)
}

func (s *Server) snmpSetAdmin(pd *store.PollDevice, ifIndex, status int) error {
	g, err := snmp.NewGoSNMP(*pd)
	if err != nil {
		return err
	}
	if err := g.Connect(); err != nil {
		return err
	}
	defer g.Conn.Close()
	return snmp.SetIfAdminStatus(g, ifIndex, status)
}

func (s *Server) snmpGetAdmin(pd *store.PollDevice, ifIndex int) (int, error) {
	g, err := snmp.NewGoSNMP(*pd)
	if err != nil {
		return 0, err
	}
	if err := g.Connect(); err != nil {
		return 0, err
	}
	defer g.Conn.Close()
	return snmp.GetIfAdminStatus(g, ifIndex)
}

func (s *Server) sshPortChange(ctx context.Context, deviceID int64, ifIndex int, descr *string, adminUp *bool) error {
	return s.sshPortChangeEx(ctx, deviceID, ifIndex, descr, adminUp, nil, nil, nil)
}

func (s *Server) sshPortChangeEx(ctx context.Context, deviceID int64, ifIndex int, descr *string, adminUp *bool, poeMode *string, isolate *bool, flowControl *bool) error {
	if err := s.guardMikrotikRouterPortWrite(ctx, deviceID); err != nil {
		return err
	}
	creds, iface, err := s.resolvePortSSH(ctx, deviceID, ifIndex)
	if err != nil {
		return err
	}
	if err := swcfg.ApplyPortChange(creds, swcfg.PortChange{
		Interface:   iface,
		Description: descr,
		AdminUp:     adminUp,
		PoEMode:     poeMode,
		Isolate:     isolate,
		FlowControl: flowControl,
		Write:       true,
	}); err != nil {
		return err
	}
	s.invalidateDeviceRunningConfig(deviceID)
	return nil
}

type patchPortPoEBody struct {
	PoEMode string `json:"poe_mode"`
}

func (s *Server) handlePatchPortPoE(w http.ResponseWriter, r *http.Request) {
	if !s.requireOperatorOrAdmin(w, r) {
		return
	}
	deviceID, ifIndex, ok := parseDeviceIfIndex(w, r)
	if !ok {
		return
	}
	var body patchPortPoEBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON")
		return
	}
	mode, err := swcfg.NormalizePoEMode(body.PoEMode)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	via, err := s.pushPortPoE(r.Context(), deviceID, ifIndex, mode)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	s.audit(r, "port.poe.update", "device", &deviceID, map[string]interface{}{
		"if_index": ifIndex,
		"poe_mode": mode,
		"via":      via,
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok": true, "poe_mode": mode, "via": via,
	})
}

func (s *Server) pushPortPoE(ctx context.Context, deviceID int64, ifIndex int, mode string) (via string, err error) {
	pd, err := s.st.GetPollDevice(ctx, deviceID)
	if err != nil {
		return "", fmt.Errorf("устройство: %w", err)
	}
	if pd == nil {
		return "", store.ErrDeviceNotFound
	}
	// v1: SSH first (EdgeSwitch Fastpath). SNMP SET PoE пока нет.
	m := mode
	sshErr := s.sshPortChangeEx(ctx, deviceID, ifIndex, nil, nil, &m, nil, nil)
	if sshErr == nil {
		return "ssh", nil
	}
	return "", fmt.Errorf("не удалось сменить PoE Mode (SSH: %v)", sshErr)
}

type patchPortIsolateBody struct {
	Isolate bool `json:"isolate"`
}

func (s *Server) handlePatchPortIsolate(w http.ResponseWriter, r *http.Request) {
	if !s.requireOperatorOrAdmin(w, r) {
		return
	}
	deviceID, ifIndex, ok := parseDeviceIfIndex(w, r)
	if !ok {
		return
	}
	var body patchPortIsolateBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON")
		return
	}
	via, err := s.pushPortIsolate(r.Context(), deviceID, ifIndex, body.Isolate)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	s.audit(r, "port.isolate.update", "device", &deviceID, map[string]interface{}{
		"if_index": ifIndex,
		"isolate":  body.Isolate,
		"via":      via,
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok": true, "isolate": body.Isolate, "via": via,
	})
}

func (s *Server) pushPortIsolate(ctx context.Context, deviceID int64, ifIndex int, isolate bool) (via string, err error) {
	pd, err := s.st.GetPollDevice(ctx, deviceID)
	if err != nil {
		return "", fmt.Errorf("устройство: %w", err)
	}
	if pd == nil {
		return "", store.ErrDeviceNotFound
	}
	iso := isolate
	sshErr := s.sshPortChangeEx(ctx, deviceID, ifIndex, nil, nil, nil, &iso, nil)
	if sshErr == nil {
		return "ssh", nil
	}
	return "", fmt.Errorf("не удалось сменить Isolate (SSH: %v)", sshErr)
}

type patchPortFlowControlBody struct {
	FlowControl bool `json:"flow_control"`
}

func (s *Server) handlePatchPortFlowControl(w http.ResponseWriter, r *http.Request) {
	if !s.requireOperatorOrAdmin(w, r) {
		return
	}
	deviceID, ifIndex, ok := parseDeviceIfIndex(w, r)
	if !ok {
		return
	}
	var body patchPortFlowControlBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON")
		return
	}
	via, err := s.pushPortFlowControl(r.Context(), deviceID, ifIndex, body.FlowControl)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	s.audit(r, "port.flow_control.update", "device", &deviceID, map[string]interface{}{
		"if_index":      ifIndex,
		"flow_control":  body.FlowControl,
		"via":           via,
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok": true, "flow_control": body.FlowControl, "via": via,
	})
}

func (s *Server) pushPortFlowControl(ctx context.Context, deviceID int64, ifIndex int, on bool) (via string, err error) {
	pd, err := s.st.GetPollDevice(ctx, deviceID)
	if err != nil {
		return "", fmt.Errorf("устройство: %w", err)
	}
	if pd == nil {
		return "", store.ErrDeviceNotFound
	}
	fc := on
	sshErr := s.sshPortChangeEx(ctx, deviceID, ifIndex, nil, nil, nil, nil, &fc)
	if sshErr == nil {
		return "ssh", nil
	}
	return "", fmt.Errorf("не удалось сменить Flow Control (SSH: %v)", sshErr)
}

type patchPortSTPBody struct {
	Enabled      *bool   `json:"enabled"`
	EdgePort     *string `json:"edge_port"`
	PortPriority *int    `json:"port_priority"`
	PathCost     *int    `json:"path_cost"`
}

func (s *Server) handlePatchPortSTP(w http.ResponseWriter, r *http.Request) {
	if !s.requireOperatorOrAdmin(w, r) {
		return
	}
	deviceID, ifIndex, ok := parseDeviceIfIndex(w, r)
	if !ok {
		return
	}
	var body patchPortSTPBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON")
		return
	}
	if body.Enabled == nil && body.EdgePort == nil && body.PortPriority == nil && body.PathCost == nil {
		writeError(w, http.StatusBadRequest, "нечего менять")
		return
	}
	via, err := s.pushPortSTP(r.Context(), deviceID, ifIndex, body)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	s.audit(r, "port.stp.update", "device", &deviceID, map[string]interface{}{
		"if_index": ifIndex, "body": body, "via": via,
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "via": via})
}

func (s *Server) pushPortSTP(ctx context.Context, deviceID int64, ifIndex int, body patchPortSTPBody) (via string, err error) {
	if err := s.guardMikrotikRouterPortWrite(ctx, deviceID); err != nil {
		return "", err
	}
	pd, err := s.st.GetPollDevice(ctx, deviceID)
	if err != nil {
		return "", fmt.Errorf("устройство: %w", err)
	}
	if pd == nil {
		return "", store.ErrDeviceNotFound
	}
	creds, iface, err := s.resolvePortSSH(ctx, deviceID, ifIndex)
	if err != nil {
		return "", err
	}
	st := swcfg.STPChange{
		Enabled:      body.Enabled,
		EdgePort:     body.EdgePort,
		PortPriority: body.PortPriority,
		PathCost:     body.PathCost,
	}
	if err := swcfg.ApplyPortChange(creds, swcfg.PortChange{Interface: iface, STP: &st, Write: true}); err != nil {
		return "", fmt.Errorf("не удалось сменить STP (SSH: %v)", err)
	}
	s.invalidateDeviceRunningConfig(deviceID)
	return "ssh", nil
}

type patchPortDHCPSnoopBody struct {
	Trusted bool `json:"trusted"`
}

func (s *Server) handlePatchPortDHCPSnoop(w http.ResponseWriter, r *http.Request) {
	if !s.requireOperatorOrAdmin(w, r) {
		return
	}
	deviceID, ifIndex, ok := parseDeviceIfIndex(w, r)
	if !ok {
		return
	}
	var body patchPortDHCPSnoopBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON")
		return
	}
	via, err := s.pushPortDHCPSnoop(r.Context(), deviceID, ifIndex, body.Trusted)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	s.audit(r, "port.dhcp_snooping.update", "device", &deviceID, map[string]interface{}{
		"if_index": ifIndex,
		"trusted":  body.Trusted,
		"via":      via,
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok": true, "trusted": body.Trusted, "via": via,
	})
}

func (s *Server) pushPortDHCPSnoop(ctx context.Context, deviceID int64, ifIndex int, trusted bool) (via string, err error) {
	if err := s.guardMikrotikRouterPortWrite(ctx, deviceID); err != nil {
		return "", err
	}
	pd, err := s.st.GetPollDevice(ctx, deviceID)
	if err != nil {
		return "", fmt.Errorf("устройство: %w", err)
	}
	if pd == nil {
		return "", store.ErrDeviceNotFound
	}
	creds, iface, err := s.resolvePortSSH(ctx, deviceID, ifIndex)
	if err != nil {
		return "", err
	}
	if err := swcfg.ApplyDHCPSnoopingTrust(creds, iface, trusted); err != nil {
		return "", fmt.Errorf("не удалось сменить DHCP Snooping trust (SSH: %v)", err)
	}
	s.invalidateDeviceRunningConfig(deviceID)
	return "ssh", nil
}

func (s *Server) handleGetPortSettings(w http.ResponseWriter, r *http.Request) {
	deviceID, ifIndex, ok := parseDeviceIfIndex(w, r)
	if !ok {
		return
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
	adminUp := true
	adminVia := "poll"
	if found.AdminStatus != nil {
		adminUp = *found.AdminStatus == 1
	}
	// SNMP ifAdminStatus — источник истины для «вкл/выкл»: CLI running-config после
	// SNMP SET часто не содержит shutdown, а ParseInterfaceConfigSnippet тогда врёт «включён».
	if pd, err := s.st.GetPollDevice(r.Context(), deviceID); err == nil && pd != nil {
		if st, gerr := s.snmpGetAdmin(pd, ifIndex); gerr == nil && (st == 1 || st == 2) {
			adminUp = st == 1
			adminVia = "snmp"
			if found.AdminStatus == nil || *found.AdminStatus != st {
				_ = s.st.UpdateInterfaceAdminStatus(r.Context(), deviceID, ifIndex, st)
			}
		}
	}
	resp := map[string]interface{}{
		"if_index":       ifIndex,
		"if_name":        found.IfName,
		"admin_up":       adminUp,
		"admin_via":      adminVia,
		"isolate":        false,
		"poe_mode":       "poe+",
		"dhcp_trusted":   false,
		"flow_control":   false,
		"stp_enabled":    true,
		"edge_port":      "auto",
		"port_priority":  128,
		"path_cost":      0,
		"via":            "poll",
		"live":           false,
		"live_err":       nil,
		"poe_24v":        false,
	}
	if found.CliAccessVlan != nil {
		resp["access_vlan"] = *found.CliAccessVlan
	}
	if found.CLIPortMode != nil && strings.TrimSpace(*found.CLIPortMode) != "" {
		resp["port_mode"] = strings.TrimSpace(*found.CLIPortMode)
	}
	if s.isMikrotikRouterDevice(r.Context(), deviceID) {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	creds, iface, sshErr := s.resolvePortSSH(r.Context(), deviceID, ifIndex)
	if sshErr != nil {
		resp["live_err"] = sshErr.Error()
		writeJSON(w, http.StatusOK, resp)
		return
	}
	v := swcfg.DetectVendor(string(creds.Vendor), creds.SysDescr, creds.Name)
	resp["poe_24v"] = swcfg.SupportsPoE24V(v, creds.SysDescr, creds.Name)

	forceLive := r.URL.Query().Get("force") == "1" || strings.EqualFold(r.URL.Query().Get("force"), "true")
	if !forceLive {
		if cfgText, via, ok := s.runningConfigForPortSettings(r.Context(), deviceID); ok {
			if live, found := swcfg.ParseInterfaceFromRunningConfig(cfgText, iface); found {
				applyPortLiveSettings(resp, live, via)
				writeJSON(w, http.StatusOK, resp)
				return
			}
		}
	}

	live, _, liveErr := swcfg.FetchPortLiveSettings(creds, iface)
	if liveErr != nil {
		resp["live_err"] = liveErr.Error()
		writeJSON(w, http.StatusOK, resp)
		return
	}
	applyPortLiveSettings(resp, live, "ssh")
	writeJSON(w, http.StatusOK, resp)
}

func applyPortLiveSettings(resp map[string]interface{}, live swcfg.PortLiveSettings, via string) {
	// admin_up оставляем из SNMP/poll — не перезаписываем CLI-дефолтом AdminUp=true
	resp["isolate"] = live.Isolate
	resp["poe_mode"] = live.PoEMode
	resp["dhcp_trusted"] = live.DHCPTrusted
	resp["flow_control"] = live.FlowControl
	resp["stp_enabled"] = live.STPEnabled
	resp["edge_port"] = live.EdgePort
	resp["port_priority"] = live.PortPriority
	resp["path_cost"] = live.PathCost
	if live.AccessVLAN != nil {
		resp["access_vlan"] = *live.AccessVLAN
	}
	if live.PortMode != "" {
		resp["port_mode"] = live.PortMode
	}
	resp["via"] = via
	resp["live"] = true
	resp["live_err"] = nil
}

func (s *Server) resolveDeviceSSH(ctx context.Context, deviceID int64) (swcfg.Creds, error) {
	dev, err := s.st.GetDevice(ctx, deviceID)
	if err != nil {
		return swcfg.Creds{}, err
	}
	if dev == nil {
		return swcfg.Creds{}, store.ErrDeviceNotFound
	}
	bs, _ := s.st.GetBackupSettings(ctx)
	user, pass, enable, port, timeout := portrolesync.ResolveDeviceSSH(dev, bs, s.cfg)
	if user == "" || pass == "" {
		return swcfg.Creds{}, fmt.Errorf("нет SSH-логина/пароля (карточка узла, бэкап или SSH_POE_*)")
	}
	sys := ""
	if dev.SysDescr != nil {
		sys = *dev.SysDescr
	}
	return swcfg.Creds{
		Host:       dev.Host,
		Port:       port,
		User:       user,
		Password:   pass,
		EnablePass: enable,
		Vendor:     swcfg.Vendor(dev.SSHVendor),
		SysDescr:   sys,
		Name:       dev.Name,
		Timeout:    timeout,
		KnownHosts: backup.KnownHostsPath(s.cfg),
	}, nil
}

func (s *Server) resolvePortSSH(ctx context.Context, deviceID int64, ifIndex int) (swcfg.Creds, string, error) {
	creds, err := s.resolveDeviceSSH(ctx, deviceID)
	if err != nil {
		return swcfg.Creds{}, "", err
	}
	iface, err := s.st.GetInterfaceName(ctx, deviceID, ifIndex)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return swcfg.Creds{}, "", store.ErrDeviceNotFound
		}
		return swcfg.Creds{}, "", err
	}
	return creds, iface, nil
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func (s *Server) isMikrotikRouterDevice(ctx context.Context, deviceID int64) bool {
	dev, err := s.st.GetDevice(ctx, deviceID)
	if err != nil || dev == nil {
		return false
	}
	return swcfg.IsMikrotikRouterDevice(dev.DeviceCategory, dev.SSHVendor, derefStr(dev.SysDescr), dev.Name)
}

func (s *Server) guardMikrotikRouterPortWrite(ctx context.Context, deviceID int64) error {
	if s.isMikrotikRouterDevice(ctx, deviceID) {
		return fmt.Errorf("RouterOS-роутер MikroTik: управление портами по SSH отключено (только просмотр)")
	}
	return nil
}
