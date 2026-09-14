package api

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/investigate"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/swcfg"
	"github.com/go-chi/chi/v5"
)

// matchVLANInInventory — совместимость со старыми тестами API.
func matchVLANInInventory(inv []swcfg.VLANInventoryRow, vlanID int) (ok bool, inDB, onPorts bool, name string) {
	return investigate.MatchVLANInInventory(inv, vlanID)
}

func parseVLANIDsQuery(raw string) ([]int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("укажите vlan_ids")
	}
	parts := swcfg.ParseVLANIDList(swcfg.NormalizeVLANList(raw))
	if len(parts) == 0 {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 4094 {
			return nil, fmt.Errorf("неверный vlan_ids")
		}
		parts = []int{n}
	}
	seen := map[int]struct{}{}
	out := make([]int, 0, len(parts))
	for _, id := range parts {
		if id <= 1 || id > 4094 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("нет VLAN для проверки (кроме VLAN 1)")
	}
	sort.Ints(out)
	return out, nil
}

// GET /devices/{id}/vlans/delete-impact?vlan_ids=14,15
func (s *Server) handleVLANDeleteImpact(w http.ResponseWriter, r *http.Request) {
	deviceID, err := parseDeviceID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "неверный id")
		return
	}
	vlanIDs, err := parseVLANIDsQuery(r.URL.Query().Get("vlan_ids"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
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
	linkPorts := map[int]investigate.TrunkPortMeta{}
	for _, p := range ifs {
		if p.IfIndex <= 0 || skipLogicalIface(p) {
			continue
		}
		name := ifaceName(p)
		linkPorts[p.IfIndex] = investigate.TrunkPortMeta{
			Name:     name,
			Override: investigate.PortDescrLinkOverride(name, p.IfDescr, p.CLIDescription, p.DescrOverride),
		}
	}

	_, localInv, _ := s.deviceVLANInventory(r.Context(), deviceID)

	neighbors, err := s.st.ListPortNeighbors(r.Context(), deviceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	neighbors, err = s.st.EnrichNeighborsRemoteDeviceID(r.Context(), neighbors)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	edges := make([]investigate.BlastNeighborEdge, 0)
	for _, n := range neighbors {
		if n.Stale || n.Protocol == store.NeighborProtocolFDB {
			continue
		}
		if _, ok := linkPorts[n.IfIndex]; !ok {
			continue
		}
		var remoteID int64
		if n.RemoteDeviceID != nil && *n.RemoteDeviceID > 0 {
			remoteID = *n.RemoteDeviceID
		}
		if remoteID == 0 && n.RemoteChassisID != nil {
			if id, _, okFind, _ := s.st.FindDeviceByChassisMAC(r.Context(), *n.RemoteChassisID); okFind {
				remoteID = id
			}
		}
		if remoteID == 0 && n.RemoteMgmtAddr != nil && strings.TrimSpace(*n.RemoteMgmtAddr) != "" {
			if id, _, okFind, _ := s.st.FindDeviceByHost(r.Context(), *n.RemoteMgmtAddr); okFind {
				remoteID = id
			}
		}
		// RemoteID=0 → AnalyzeVLANDeleteBlast запишет skip unresolved_neighbor
		if remoteID == deviceID {
			remoteID = 0
		}
		edges = append(edges, investigate.BlastNeighborEdge{
			IfIndex:       n.IfIndex,
			Protocol:      n.Protocol,
			RemoteID:      remoteID,
			RemotePortID:  n.RemotePortID,
			RemoteSysName: n.RemoteSysName,
		})
	}

	var rootID int64
	var rootSource string
	var dist map[int64]int
	var adj map[int64][]int64
	var edgeCarries func(a, b int64, vlanID int) bool
	hasEdgeVLANs := false
	topoSource := "live"
	if cache, ok, cerr := s.st.LoadTopologyBlastCache(r.Context()); cerr == nil && ok && cache != nil {
		rootID = cache.RootDeviceID
		rootSource = cache.RootSource
		adj = cache.Adj
		dist = cache.Dist
		topoSource = "cache"
		hasEdgeVLANs = cache.HasEdgeVLANs()
		if hasEdgeVLANs {
			c := cache
			edgeCarries = func(a, b int64, vlanID int) bool {
				return c.EdgeCarriesVLAN(a, b, vlanID)
			}
		}
	} else {
		includeStale := false
		g, gerr := s.st.BuildTopologyGraphFiltered(r.Context(), store.TopologyFilter{
			Protocol:     "lldp",
			Dedup:        true,
			IncludeStale: &includeStale,
		})
		if gerr == nil && g != nil {
			var linkCount map[int64]int
			adj, _, linkCount = investigate.BuildLLDPDeviceAdj(g)
			preferSTP := int64(0)
			if states, serr := s.st.ListDeviceSTPStates(r.Context()); serr == nil && len(states) > 0 {
				if chassis, cerr := s.st.ListChassisMACIndex(r.Context()); cerr == nil {
					hexMap := map[string]int64{}
					for mac, ep := range chassis {
						hexMap[mac] = ep.ID
					}
					preferSTP = investigate.PreferSTPRootDeviceID(states, hexMap)
				}
			}
			rootID, rootSource = investigate.PickTopologyRootIDPrefer(g.Nodes, linkCount, preferSTP)
			dist = investigate.BFSDistances(adj, rootID)
		}
	}

	portRoles := map[int]string{}
	ifNames := map[int]string{}
	for _, p := range ifs {
		if p.IfIndex <= 0 {
			continue
		}
		portRoles[p.IfIndex] = store.ResolveInterfacePortRole(p.PortRole, p.CLIPortMode)
		ifNames[p.IfIndex] = ifaceName(p)
	}
	var localFDB []investigate.LocalFDBHint
	if entries, ferr := s.st.ListFDBEntriesByVLANs(r.Context(), deviceID, vlanIDs); ferr == nil {
		for _, e := range entries {
			localFDB = append(localFDB, investigate.LocalFDBHint{MAC: e.MAC, IfIndex: e.IfIndex, VLANID: e.VLANID})
		}
	}

	localCfg, _ := s.deviceConfigTextForVLAN(r.Context(), deviceID)
	localSVIs := swcfg.ParseSVIAddresses(localCfg)

	wantVLAN := map[int]struct{}{}
	for _, id := range vlanIDs {
		wantVLAN[id] = struct{}{}
	}
	gateways := make([]investigate.VLANBlastGateway, 0)
	if all, lerr := s.st.ListDevices(r.Context()); lerr == nil {
		for i := range all {
			d := &all[i]
			if d.ID == deviceID {
				continue
			}
			cfg, _ := s.deviceConfigTextForVLAN(r.Context(), d.ID)
			if strings.TrimSpace(cfg) == "" {
				continue
			}
			for _, svi := range swcfg.ParseSVIAddresses(cfg) {
				if _, ok := wantVLAN[svi.VLANID]; !ok {
					continue
				}
				gateways = append(gateways, investigate.VLANBlastGateway{
					VLANID:     svi.VLANID,
					DeviceID:   d.ID,
					DeviceName: d.Name,
					Host:       d.Host,
					SVIIP:      svi.IP,
				})
			}
		}
	}

	blast := investigate.AnalyzeVLANDeleteBlast(investigate.VLANBlastInput{
		DeviceID:   deviceID,
		VLANIDs:    vlanIDs,
		RootID:     rootID,
		RootSource: rootSource,
		Dist:       dist,
		Adj:        adj,
		LocalInv:   localInv,
		TrunkPorts: linkPorts,
		Neighbors:  edges,
		LookupDevice: func(id int64) (investigate.DeviceVLANInv, bool) {
			nd, err := s.st.GetDevice(r.Context(), id)
			if err != nil || nd == nil {
				return investigate.DeviceVLANInv{}, false
			}
			_, inv, _ := s.deviceVLANInventory(r.Context(), id)
			return investigate.DeviceVLANInv{Name: nd.Name, Host: nd.Host, Inv: inv}, true
		},
		MaxDepth:        investigate.VLANBlastMaxDepth,
		LocalPortRoles:  portRoles,
		LocalIfNames:    ifNames,
		LocalFDB:        localFDB,
		LocalPortModes:  swcfg.ParseRunningConfigPortModes(localCfg),
		DeviceHost:      dev.Host,
		LocalSVIs:       localSVIs,
		Gateways:        gateways,
		EdgeCarriesVLAN: edgeCarries,
		HasEdgeVLANs:    hasEdgeVLANs,
	})

	warnings := blast.Warnings
	if warnings == nil {
		warnings = []string{}
	}
	hits := blast.Hits
	if hits == nil {
		hits = []investigate.VLANBlastHit{}
	}
	skips := blast.Skips
	if skips == nil {
		skips = []investigate.VLANBlastSkip{}
	}
	fdbClients := blast.FDBClients
	if fdbClients == nil {
		fdbClients = []investigate.VLANBlastFDBClient{}
	}
	mgmtRisks := blast.MgmtRisks
	if mgmtRisks == nil {
		mgmtRisks = []investigate.VLANBlastMgmtRisk{}
	}
	gwOut := blast.Gateways
	if gwOut == nil {
		gwOut = []investigate.VLANBlastGateway{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"device_id":           deviceID,
		"device_name":         dev.Name,
		"vlan_ids":            vlanIDs,
		"root_device_id":      blast.RootDeviceID,
		"root_source":         blast.RootSource,
		"topology_source":     topoSource,
		"neighbors":           hits,
		"skips":               skips,
		"affected_device_ids": blast.AffectedDeviceIDs,
		"toward_core_skipped": blast.TowardCoreSkipped,
		"uplink_skipped":      blast.TowardCoreSkipped,
		"redundant_hit_count": blast.RedundantHitCount,
		"fdb_clients":         fdbClients,
		"mgmt_risks":          mgmtRisks,
		"gateways":            gwOut,
		"blocks_delete":       blast.BlocksDelete,
		"severity":            blast.Severity,
		"warnings":            warnings,
		"summary":             blast.Summary,
	})
}
