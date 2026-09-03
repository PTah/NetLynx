package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/snmp"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
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
		writeError(w, http.StatusBadRequest, errMsg)
		return
	}
	seen, err := s.st.HasPortFDBEntry(r.Context(), deviceID, ifIndex, mac)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !seen {
		writeError(w, http.StatusBadRequest, "этот MAC сейчас не виден на порту (FDB)")
		return
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
			} else {
				name = mac
			}
		}
		var locPtr *string
		if loc := strings.TrimSpace(body.Location); loc != "" {
			locPtr = &loc
		}
		chassis := mac
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
			ChassisMAC:          &chassis,
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
	if err := s.st.UpsertFDBTopologyNeighbor(r.Context(), deviceID, ifIndex, mac, mgmt, sys, time.Now().UTC()); err != nil {
		writeError(w, http.StatusInternalServerError, "узел создан, но линк на топологии не записан: "+err.Error())
		return
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
	writeJSON(w, status, map[string]interface{}{
		"ok":              true,
		"id":              newID,
		"already":         already,
		"linked":          true,
		"mac":             mac,
		"device_category": cat,
	})
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
