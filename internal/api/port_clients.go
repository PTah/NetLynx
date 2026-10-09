package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/PTah/netlynx/internal/snmp"
	"github.com/PTah/netlynx/internal/store"
)

func (s *Server) handleListPortClients(w http.ResponseWriter, r *http.Request) {
	deviceID, ifIndex, ok := parseDeviceIfIndex(w, r)
	if !ok {
		return
	}
	clients, err := s.st.ListPortClients(r.Context(), deviceID, ifIndex)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if clients == nil {
		clients = []store.PortClient{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"device_id": deviceID,
		"if_index":  ifIndex,
		"clients":   clients,
	})
}

// handleRefreshPortClients — живой SNMP FDB+ARP на коммутаторе, без событий mac-move.
func (s *Server) handleRefreshPortClients(w http.ResponseWriter, r *http.Request) {
	deviceID, ifIndex, ok := parseDeviceIfIndex(w, r)
	if !ok {
		return
	}
	if !s.beginPortClientsRefresh(deviceID) {
		writeError(w, http.StatusConflict, "опрос FDB/ARP для этого устройства уже выполняется")
		return
	}
	defer s.endPortClientsRefresh(deviceID)

	pd, err := s.st.GetPollDevice(r.Context(), deviceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if pd == nil {
		writeError(w, http.StatusNotFound, "узел не найден")
		return
	}

	g, err := snmp.NewGoSNMP(*pd)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := g.Connect(); err != nil {
		writeError(w, http.StatusBadGateway, "SNMP connect: "+err.Error())
		return
	}
	defer g.Conn.Close()

	now := time.Now().UTC()
	fdbEntries, _, err := snmp.WalkFDBWithStats(g)
	if err != nil {
		_ = s.st.UpdateDeviceFDBStatus(r.Context(), deviceID, "unavailable")
		writeError(w, http.StatusBadGateway, "FDB: "+err.Error())
		return
	}
	storeFDB := make(map[string]store.FDBLearnedEntry, len(fdbEntries))
	for mac, ent := range fdbEntries {
		storeFDB[mac] = store.FDBLearnedEntry{IfIndex: ent.IfIndex, VLANID: ent.VLANID}
	}
	if err := s.st.ReplaceFDBSnapshot(r.Context(), deviceID, storeFDB, now); err != nil {
		writeError(w, http.StatusInternalServerError, "FDB snapshot: "+err.Error())
		return
	}
	if len(storeFDB) > 0 {
		_ = s.st.UpdateDeviceFDBStatus(r.Context(), deviceID, "ok")
	} else {
		_ = s.st.UpdateDeviceFDBStatus(r.Context(), deviceID, "learning")
	}

	arpCount := 0
	var arpWarning string
	if arpEntries, err := snmp.WalkARP(g); err != nil {
		arpWarning = err.Error()
	} else {
		storeARP := make([]store.ARPEntry, 0, len(arpEntries))
		for _, a := range arpEntries {
			storeARP = append(storeARP, store.ARPEntry{IP: a.IP, MAC: a.MAC, IfIndex: a.IfIndex})
		}
		if err := s.st.ReplaceARPSnapshot(r.Context(), deviceID, storeARP, now); err != nil {
			arpWarning = "ARP snapshot: " + err.Error()
		} else {
			arpCount = len(storeARP)
			_, _ = s.st.BackfillEmptyChassisFromARP(r.Context(), storeARP)
		}
	}

	clients, err := s.st.ListPortClients(r.Context(), deviceID, ifIndex)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if clients == nil {
		clients = []store.PortClient{}
	}

	s.audit(r, "port_clients.refresh", "device", &deviceID, map[string]interface{}{
		"if_index":  ifIndex,
		"fdb_count": len(storeFDB),
		"arp_count": arpCount,
		"clients":   len(clients),
	})

	resp := map[string]interface{}{
		"device_id": deviceID,
		"if_index":  ifIndex,
		"clients":   clients,
		"polled_at": now.Format(time.RFC3339),
		"fdb_count": len(storeFDB),
		"arp_count": arpCount,
	}
	if arpWarning != "" {
		resp["arp_warning"] = arpWarning
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) beginPortClientsRefresh(deviceID int64) bool {
	s.portClientsRefreshMu.Lock()
	defer s.portClientsRefreshMu.Unlock()
	if s.portClientsRefreshBusy == nil {
		s.portClientsRefreshBusy = make(map[int64]struct{})
	}
	if _, busy := s.portClientsRefreshBusy[deviceID]; busy {
		return false
	}
	s.portClientsRefreshBusy[deviceID] = struct{}{}
	return true
}

func (s *Server) endPortClientsRefresh(deviceID int64) {
	s.portClientsRefreshMu.Lock()
	defer s.portClientsRefreshMu.Unlock()
	delete(s.portClientsRefreshBusy, deviceID)
}

type promotePortClientBody struct {
	discoveredSNMPBody
	MAC string `json:"mac"`
}

func normalizePortClientMAC(raw string) (string, string) {
	mac, ok := store.FormatFullMAC(raw)
	if !ok {
		return "", "нужен полный MAC (6 октетов)"
	}
	return mac, ""
}

func (s *Server) handlePreviewPortClient(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := parseDeviceIfIndex(w, r); !ok {
		return
	}
	var body promotePortClientBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON: "+err.Error())
		return
	}
	d := syntheticDiscoveredFromClient(body.MAC, strings.TrimSpace(body.Host))
	pd, errMsg := pollDeviceFromDiscoveredBody(d, body.discoveredSNMPBody, true)
	if errMsg != "" {
		writeError(w, http.StatusBadRequest, errMsg)
		return
	}
	g, err := snmp.NewGoSNMP(pd)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	if err := g.Connect(); err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	defer g.Conn.Close()
	sysName, sysDescr, err := snmp.SysGet(g)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":        true,
		"sys_name":  sysName,
		"sys_descr": sysDescr,
		"host":      pd.Host,
	})
}

func (s *Server) handlePromotePortClient(w http.ResponseWriter, r *http.Request) {
	deviceID, ifIndex, ok := parseDeviceIfIndex(w, r)
	if !ok {
		return
	}
	parent, err := s.st.GetDevice(r.Context(), deviceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if parent == nil {
		writeError(w, http.StatusNotFound, "узел не найден")
		return
	}
	var body promotePortClientBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON: "+err.Error())
		return
	}
	mac, errMsg := normalizePortClientMAC(body.MAC)
	if errMsg != "" {
		if _, macFromHost := store.SplitHostOrMAC(strings.TrimSpace(body.Host)); macFromHost != "" {
			mac = macFromHost
			errMsg = ""
		} else if strings.TrimSpace(body.MAC) == "" {
			// Пустой FDB/ARP: узел по имени, без идентификатора на порту.
			mac = ""
			errMsg = ""
		}
	}
	if errMsg != "" {
		writeError(w, http.StatusBadRequest, errMsg)
		return
	}
	inFDB := false
	if mac != "" {
		seen, err := s.st.HasPortFDBEntry(r.Context(), deviceID, ifIndex, mac)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		inFDB = seen
	}

	d := syntheticDiscoveredFromClient(mac, strings.TrimSpace(body.Host))
	pd, errMsg := pollDeviceFromDiscoveredBody(d, body.discoveredSNMPBody, false)
	if errMsg != "" {
		writeError(w, http.StatusBadRequest, errMsg)
		return
	}

	existingID, existingName, found, err := s.st.FindDeviceByChassisMAC(r.Context(), mac)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !found && pd.Host != "" {
		existingID, existingName, found, err = s.st.FindDeviceByHost(r.Context(), pd.Host)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if found && existingID == deviceID {
		writeError(w, http.StatusBadRequest, "нельзя связать свитч сам с собой")
		return
	}

	already := found
	newID := existingID
	if !found {
		name := strings.TrimSpace(body.Name)
		if name == "" {
			if pd.Host != "" {
				name = pd.Host
			} else if mac != "" {
				name = mac
			} else {
				writeError(w, http.StatusBadRequest, "укажите имя узла (на порту нет MAC и IP)")
				return
			}
		}
		var locPtr *string
		if loc := strings.TrimSpace(body.Location); loc != "" {
			locPtr = &loc
		}
		var chassisPtr *string
		if mac != "" {
			chassis := mac
			chassisPtr = &chassis
		}
		newID, err = s.st.CreateDevice(r.Context(), store.CreateDeviceInput{
			Name:                name,
			Host:                pd.Host,
			Location:            locPtr,
			DeviceCategory:      body.DeviceCategory,
			SNMPVersion:         pd.SNMPVersion,
			Community:           pd.Community,
			V3User:              pd.V3User,
			V3AuthProtocol:      pd.V3AuthProtocol,
			V3AuthPass:          pd.V3AuthPass,
			V3PrivProtocol:      pd.V3PrivProtocol,
			V3PrivPass:          pd.V3PrivPass,
			V3EngineID:          pd.V3EngineID,
			PollIntervalSeconds: pd.PollIntervalSeconds,
			ChassisMAC:          chassisPtr,
		})
		if err != nil {
			if dup, ok := store.IsDuplicateIdentity(err); ok {
				writeError(w, http.StatusConflict, dup.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		existingName = name
	}

	var mgmt *string
	if pd.Host != "" {
		h := pd.Host
		mgmt = &h
	}
	var sys *string
	if n := strings.TrimSpace(body.Name); n != "" {
		sys = &n
	} else if existingName != "" {
		sys = &existingName
	}
	if inFDB && mac != "" {
		if err := s.st.UpsertFDBTopologyNeighbor(r.Context(), deviceID, ifIndex, mac, mgmt, sys, time.Now().UTC()); err != nil {
			writeError(w, http.StatusInternalServerError, "узел создан, но линк на топологии не записан: "+err.Error())
			return
		}
	}

	cat := store.NormalizeDeviceCategory(body.DeviceCategory)
	s.audit(r, "port_client.promote", "device", &newID, map[string]interface{}{
		"from_device_id":  deviceID,
		"from_if_index":   ifIndex,
		"mac":             mac,
		"host":            pd.Host,
		"already":         already,
		"device_category": cat,
	})
	status := http.StatusCreated
	if already {
		status = http.StatusOK
	}
	resp := map[string]interface{}{
		"ok":              true,
		"id":              newID,
		"already":         already,
		"linked":          inFDB,
		"mac":             mac,
		"device_category": cat,
	}
	if !already {
		if warn := s.onboardSSHDevice(r.Context(), newID); warn != "" {
			resp["ssh_warning"] = warn
		}
	}
	writeJSON(w, status, resp)
}

func syntheticDiscoveredFromClient(mac, host string) *store.DiscoveredDevice {
	d := &store.DiscoveredDevice{}
	if mac != "" {
		m := mac
		d.RemoteChassisID = &m
	}
	if host != "" {
		h := host
		d.RemoteMgmtAddr = &h
	}
	return d
}
