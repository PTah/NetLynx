package api

import (
	"net/http"
	"strconv"
	"strings"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/models"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/snmp"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

// GET /devices/{id}/interfaces/{ifIndex}/shut-impact
// Превью: кто/что может отвалиться при shutdown порта (FDB + LLDP/CDP + риски uplink).
func (s *Server) handlePortShutImpact(w http.ResponseWriter, r *http.Request) {
	deviceID, ifIndex, ok := parseDeviceIfIndex(w, r)
	if !ok {
		return
	}
	dev, err := s.st.GetDevice(r.Context(), deviceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if dev == nil {
		writeError(w, http.StatusNotFound, "узел не найден")
		return
	}

	ifs, err := s.st.ListInterfacesByDevice(r.Context(), deviceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var ifName, ifDescr, portRole string
	var admin, oper *int
	foundPort := false
	for i := range ifs {
		if ifs[i].IfIndex != ifIndex {
			continue
		}
		foundPort = true
		row := ifs[i]
		ifName = strings.TrimSpace(derefStr(row.IfName))
		ifDescr = portDisplayDescr(row)
		portRole = strings.TrimSpace(row.PortRole)
		admin = row.AdminStatus
		oper = row.OperStatus
		break
	}
	if !foundPort {
		writeError(w, http.StatusNotFound, "порт не найден")
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

	neighbors, err := s.st.ListPortNeighborsOnIfaces(r.Context(), deviceID, []int{ifIndex})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if neighbors == nil {
		neighbors = []store.PortNeighbor{}
	}

	type neighborImpact struct {
		Protocol         string  `json:"protocol"`
		RemoteSysName    *string `json:"remote_sys_name,omitempty"`
		RemotePortID     *string `json:"remote_port_id,omitempty"`
		RemoteMgmtAddr   *string `json:"remote_mgmt_addr,omitempty"`
		RemoteChassisID  *string `json:"remote_chassis_id,omitempty"`
		RemoteDeviceID   *int64  `json:"remote_device_id,omitempty"`
		RemoteDeviceName *string `json:"remote_device_name,omitempty"`
		LooksLikeInfra   bool    `json:"looks_like_infra"`
	}

	nOut := make([]neighborImpact, 0, len(neighbors))
	infraNeighbor := false
	inventoryNeighbor := false
	for _, n := range neighbors {
		if n.Protocol == store.NeighborProtocolFDB {
			continue
		}
		ni := neighborImpact{
			Protocol:        n.Protocol,
			RemoteSysName:   n.RemoteSysName,
			RemotePortID:    n.RemotePortID,
			RemoteMgmtAddr:  n.RemoteMgmtAddr,
			RemoteChassisID: n.RemoteChassisID,
		}
		info := snmp.NeighborInfo{
			RemoteSysName:   derefStr(n.RemoteSysName),
			RemotePortID:    derefStr(n.RemotePortID),
			RemoteChassisID: derefStr(n.RemoteChassisID),
			RemoteMgmtAddr:  derefStr(n.RemoteMgmtAddr),
		}
		ni.LooksLikeInfra = snmp.IsInfraLLDPNeighbor(info)
		if ni.LooksLikeInfra {
			infraNeighbor = true
		}
		if n.RemoteChassisID != nil {
			if id, name, okFind, _ := s.st.FindDeviceByChassisMAC(r.Context(), *n.RemoteChassisID); okFind {
				ni.RemoteDeviceID = &id
				ni.RemoteDeviceName = &name
				inventoryNeighbor = true
			}
		}
		if ni.RemoteDeviceID == nil && n.RemoteMgmtAddr != nil && strings.TrimSpace(*n.RemoteMgmtAddr) != "" {
			if id, name, okFind, _ := s.st.FindDeviceByHost(r.Context(), *n.RemoteMgmtAddr); okFind {
				ni.RemoteDeviceID = &id
				ni.RemoteDeviceName = &name
				inventoryNeighbor = true
			}
		}
		nOut = append(nOut, ni)
	}

	macsOnPort := len(clients)
	role := strings.ToLower(strings.TrimSpace(portRole))
	if role == "" {
		role = "access"
	}
	uplinkSuspected := role == "trunk" || macsOnPort >= 8 || infraNeighbor || inventoryNeighbor

	warnings := make([]string, 0, 5)
	if role == "trunk" {
		warnings = append(warnings, "Роль порта trunk — похоже на uplink; shutdown может отрезать сегмент или доступ к свитчу.")
	}
	if macsOnPort >= 8 {
		warnings = append(warnings, "На порту много MAC (≥8) — часто uplink/хаб, а не один endpoint.")
	}
	if infraNeighbor || inventoryNeighbor {
		warnings = append(warnings, "За портом виден другой коммутатор (LLDP/CDP) — риск потери связи с ним и со всем, что за ним.")
	}
	if uplinkSuspected {
		warnings = append(warnings, "Высокий риск: можете потерять SNMP/SSH к этому свитчу, если management идёт через этот линк.")
	}
	if macsOnPort == 0 && len(nOut) == 0 {
		warnings = append(warnings, "FDB и LLDP пусты — последствия неизвестны (снимок мог быть пуст).")
	}

	severity := "info"
	if uplinkSuspected {
		severity = "critical"
	} else if macsOnPort > 0 || len(nOut) > 0 {
		severity = "warning"
	}

	clientOut := make([]map[string]interface{}, 0, len(clients))
	for _, c := range clients {
		row := map[string]interface{}{
			"mac":        c.MAC,
			"mac_vendor": c.MacVendor,
			"last_seen":  c.LastSeenAt,
		}
		if c.VLANID != nil {
			row["vlan_id"] = *c.VLANID
		}
		if len(c.IPs) > 0 {
			row["ips"] = c.IPs
		} else if c.IP != nil {
			row["ips"] = []string{*c.IP}
		}
		if c.ExistingDeviceID != nil {
			row["device_id"] = *c.ExistingDeviceID
			if c.ExistingDeviceName != nil {
				row["device_name"] = *c.ExistingDeviceName
			}
		}
		clientOut = append(clientOut, row)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"device_id":        deviceID,
		"device_name":      dev.Name,
		"device_host":      dev.Host,
		"if_index":         ifIndex,
		"if_name":          ifName,
		"if_descr":         ifDescr,
		"port_role":        role,
		"admin_status":     admin,
		"oper_status":      oper,
		"macs_on_port":     macsOnPort,
		"clients":          clientOut,
		"neighbors":        nOut,
		"uplink_suspected": uplinkSuspected,
		"severity":          severity,
		"warnings":         warnings,
		"summary":          shutImpactSummary(macsOnPort, len(nOut), uplinkSuspected),
	})
}

func portDisplayDescr(row models.DeviceInterface) string {
	if ov := strings.TrimSpace(derefStr(row.DescrOverride)); ov != "" {
		return ov
	}
	if cli := strings.TrimSpace(derefStr(row.CLIDescription)); cli != "" {
		return cli
	}
	return strings.TrimSpace(derefStr(row.IfDescr))
}

func shutImpactSummary(macs, neigh int, uplink bool) string {
	if uplink {
		return "Похоже на uplink: shutdown может отрезать свитч или сегмент за ним."
	}
	if macs == 0 && neigh == 0 {
		return "Явных устройств за портом в снимке нет."
	}
	var b strings.Builder
	if macs > 0 {
		b.WriteString(strconv.Itoa(macs))
		b.WriteString(" MAC в FDB")
	}
	if neigh > 0 {
		if b.Len() > 0 {
			b.WriteString(", ")
		}
		b.WriteString(strconv.Itoa(neigh))
		b.WriteString(" LLDP/CDP сосед(ей)")
	}
	b.WriteString(" — при shutdown потеряют линк.")
	return b.String()
}
