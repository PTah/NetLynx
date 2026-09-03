package investigate

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/macvendor"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

type Confidence string

const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
	ConfidenceLow    Confidence = "low"
)

type Hypothesis struct {
	ID              string     `json:"id"`
	Confidence      Confidence `json:"confidence"`
	Title           string     `json:"title"`
	Evidence        []string   `json:"evidence"`
	SuggestedChecks []string   `json:"suggested_checks"`
}

type FootprintEntry struct {
	store.MACFootprintHit
	Neighbors []NeighborBrief `json:"neighbors,omitempty"`
}

type NeighborBrief struct {
	Protocol        string  `json:"protocol"`
	RemoteSysName   *string `json:"remote_sys_name,omitempty"`
	RemotePortID    *string `json:"remote_port_id,omitempty"`
	RemoteChassisID *string `json:"remote_chassis_id,omitempty"`
	RemoteMgmtAddr  *string `json:"remote_mgmt_addr,omitempty"`
}

type TimelineEntry struct {
	store.MACFDBMove
	DeviceName string `json:"device_name,omitempty"`
	DeviceHost string `json:"device_host,omitempty"`
}

type Identity struct {
	MAC              string   `json:"mac"`
	Vendor           string   `json:"vendor,omitempty"`
	LocallyAdmin     bool     `json:"locally_administered"`
	VirtualizationHint bool   `json:"virtualization_hint"`
	IPs              []string `json:"ips,omitempty"`
	InventoryDeviceID *int64  `json:"inventory_device_id,omitempty"`
	InventoryName    *string  `json:"inventory_name,omitempty"`
}

type Report struct {
	Identity      Identity                 `json:"identity"`
	Investigation store.MACInvestigationStatus `json:"investigation"`
	Hypotheses    []Hypothesis             `json:"hypotheses"`
	Timeline    []TimelineEntry    `json:"timeline"`
	Footprint   []FootprintEntry   `json:"footprint"`
	FDBHistory  []FDBHistoryPoint  `json:"fdb_history,omitempty"`
	L2Paths     []L2Path           `json:"l2_paths,omitempty"`
	MoveGraph   MoveGraph          `json:"move_graph"`
	Correlated  []store.EventBrief `json:"correlated_events"`
	WiFiUntracked bool               `json:"wifi_untracked,omitempty"`
	WiFiUntrackedNote string           `json:"wifi_untracked_note,omitempty"`
	GeneratedAt time.Time          `json:"generated_at"`
}

type FDBHistoryPoint struct {
	DaysAgo  int                       `json:"days_ago"`
	TargetAt time.Time                 `json:"target_at"`
	Hits     []store.FDBSnapshotMACHit `json:"hits"`
}

type Builder struct {
	St *store.Store
}

func (b *Builder) BuildMACReport(ctx context.Context, rawMAC string) (*Report, error) {
	mac, ok := store.FormatFullMAC(rawMAC)
	if !ok {
		return nil, ErrInvalidMAC
	}
	skip, err := b.St.ShouldSkipWiFiMACTracking(ctx, mac)
	if err != nil {
		return nil, err
	}
	if skip {
		return nil, ErrWiFiMACNotTracked
	}

	now := time.Now()
	rep := &Report{
		Identity:    buildIdentity(ctx, b.St, mac),
		GeneratedAt: now,
		Hypotheses:  []Hypothesis{},
		Timeline:    []TimelineEntry{},
		Footprint:   []FootprintEntry{},
		FDBHistory:  []FDBHistoryPoint{},
		L2Paths:     []L2Path{},
		Correlated:  []store.EventBrief{},
		MoveGraph:   MoveGraph{Nodes: []MoveGraphNode{}, Edges: []MoveGraphEdge{}},
	}
	if inv, err := b.St.GetMACInvestigationStatus(ctx, mac); err == nil && inv != nil {
		rep.Investigation = *inv
	} else {
		rep.Investigation = store.MACInvestigationStatus{MAC: mac, Status: store.MACInvestigationOpen}
	}

	moves, err := b.St.ListMACFDBMoves(ctx, mac, 100)
	if err != nil {
		return nil, err
	}
	devNames := map[int64]store.PollDevice{}
	if devs, err := b.St.ListPollDevices(ctx); err == nil {
		for _, d := range devs {
			devNames[d.ID] = d
		}
	}
	for _, m := range moves {
		te := TimelineEntry{MACFDBMove: m}
		if d, ok := devNames[m.DeviceID]; ok {
			te.DeviceName = d.Name
			te.DeviceHost = d.Host
		}
		rep.Timeline = append(rep.Timeline, te)
	}

	foot, err := b.St.ListMACFootprint(ctx, mac)
	if err != nil {
		return nil, err
	}
	for _, h := range foot {
		fe := FootprintEntry{MACFootprintHit: h}
		nbs, err := b.St.ListPortNeighborsOnIfaces(ctx, h.DeviceID, []int{h.IfIndex})
		if err == nil {
			for _, n := range nbs {
				if n.Protocol == store.NeighborProtocolFDB {
					continue
				}
				fe.Neighbors = append(fe.Neighbors, NeighborBrief{
					Protocol: n.Protocol, RemoteSysName: n.RemoteSysName,
					RemotePortID: n.RemotePortID, RemoteChassisID: n.RemoteChassisID,
					RemoteMgmtAddr: n.RemoteMgmtAddr,
				})
			}
		}
		rep.Footprint = append(rep.Footprint, fe)
	}

	for _, days := range []int{1, 3, 7, 14, 30} {
		at := now.AddDate(0, 0, -days)
		hits, err := b.St.LookupMACInFDBSnapshots(ctx, mac, at)
		if err != nil {
			return nil, err
		}
		if hits == nil {
			hits = []store.FDBSnapshotMACHit{}
		}
		rep.FDBHistory = append(rep.FDBHistory, FDBHistoryPoint{
			DaysAgo:  days,
			TargetAt: at,
			Hits:     hits,
		})
	}

	// корреляция по устройствам из timeline/footprint
	corrSince := now.Add(-24 * time.Hour)
	seenDev := map[int64][]int{}
	for _, h := range foot {
		seenDev[h.DeviceID] = appendUnique(seenDev[h.DeviceID], h.IfIndex)
	}
	for _, m := range moves {
		if m.FromIfIndex != nil {
			seenDev[m.DeviceID] = appendUnique(seenDev[m.DeviceID], *m.FromIfIndex)
		}
		if m.ToIfIndex != nil {
			seenDev[m.DeviceID] = appendUnique(seenDev[m.DeviceID], *m.ToIfIndex)
		}
	}
	for devID, ifs := range seenDev {
		evs, err := b.St.ListCorrelatedPortEvents(ctx, devID, ifs, corrSince, 20)
		if err != nil {
			continue
		}
		rep.Correlated = append(rep.Correlated, evs...)
	}

	// имена портов для карты (в т.ч. только из timeline)
	ifNames := collectPortNames(ctx, b.St, rep.Footprint, rep.Timeline)
	rep.MoveGraph = buildMoveGraph(rep.Timeline, rep.Footprint, ifNames)
	sortFootprint(rep.Footprint)
	rep.L2Paths = b.buildL2Paths(ctx, rep.Footprint)
	sortPathsStable(rep.L2Paths)
	rep.Hypotheses = buildHypotheses(rep)
	applyInvestigators(ctx, b.St, rep, DefaultInvestigators())
	rep.Hypotheses = preferInvestigatorIDs(rep.Hypotheses)
	return rep, nil
}

func collectPortNames(ctx context.Context, st *store.Store, foot []FootprintEntry, timeline []TimelineEntry) map[string]string {
	out := map[string]string{}
	for _, f := range foot {
		key := nodeKey(f.DeviceID, f.IfIndex)
		if f.IfName != nil && *f.IfName != "" {
			out[key] = *f.IfName
		} else if f.IfDescr != nil && *f.IfDescr != "" {
			out[key] = *f.IfDescr
		}
	}
	need := map[int64]map[int]struct{}{}
	for _, m := range timeline {
		for _, idx := range []*int{m.FromIfIndex, m.ToIfIndex} {
			if idx == nil {
				continue
			}
			k := nodeKey(m.DeviceID, *idx)
			if out[k] != "" {
				continue
			}
			if need[m.DeviceID] == nil {
				need[m.DeviceID] = map[int]struct{}{}
			}
			need[m.DeviceID][*idx] = struct{}{}
		}
	}
	for deviceID, indexes := range need {
		nameIdx, err := st.ListInterfaceNameIndex(ctx, deviceID)
		if err != nil {
			continue
		}
		for ifIndex := range indexes {
			if n := nameIdx[ifIndex]; n != "" {
				out[nodeKey(deviceID, ifIndex)] = n
			}
		}
	}
	return out
}

func buildIdentity(ctx context.Context, st *store.Store, mac string) Identity {
	id := Identity{
		MAC:                mac,
		Vendor:             macvendor.Lookup(mac),
		LocallyAdmin:       isLocallyAdministered(mac),
		VirtualizationHint: looksLikeVirtualMAC(mac),
	}
	if ips, err := st.ListARPByMAC(ctx, mac); err == nil {
		id.IPs = ips
	}
	if chassis, err := st.ListChassisMACIndex(ctx); err == nil {
		hex := strings.ReplaceAll(mac, ":", "")
		if ep, ok := chassis[hex]; ok {
			did := ep.ID
			name := ep.Name
			id.InventoryDeviceID = &did
			id.InventoryName = &name
		}
	}
	return id
}

func isLocallyAdministered(mac string) bool {
	h := strings.ReplaceAll(strings.ToLower(mac), ":", "")
	if len(h) < 2 {
		return false
	}
	b, err := parseHexByte(h[0:2])
	if err != nil {
		return false
	}
	return b&0x02 != 0
}

func parseHexByte(s string) (byte, error) {
	var v byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		var n byte
		switch {
		case c >= '0' && c <= '9':
			n = c - '0'
		case c >= 'a' && c <= 'f':
			n = c - 'a' + 10
		case c >= 'A' && c <= 'F':
			n = c - 'A' + 10
		default:
			return 0, errInvalid
		}
		v = v<<4 | n
	}
	return v, nil
}

var errInvalid = errString("invalid")

type errString string

func (e errString) Error() string { return string(e) }

var ErrInvalidMAC = errString("invalid mac")

// ErrWiFiMACNotTracked — MAC WiFi-клиента при выключенном track_wifi_clients.
var ErrWiFiMACNotTracked = errString("wifi mac not tracked")

func looksLikeVirtualMAC(mac string) bool {
	h := strings.ReplaceAll(strings.ToLower(mac), ":", "")
	if len(h) < 6 {
		return false
	}
	pfx := h[:6]
	// QEMU/KVM 52:54:00…, часто кастомные 52:54:xx; VMware 00:50:56; Hyper-V 00:15:5d; Xen 00:16:3e
	switch {
	case strings.HasPrefix(h, "5254"):
		return true
	case pfx == "005056", pfx == "000c29", pfx == "00155d", pfx == "00163e":
		return true
	}
	return isLocallyAdministered(mac) && (strings.HasPrefix(h, "52") || strings.HasPrefix(h, "02") || strings.HasPrefix(h, "06"))
}

// hypervisorLabel возвращает читаемое имя гипервизора по OUI/LAA или пустую строку.
func hypervisorLabel(mac string) string {
	h := strings.ReplaceAll(strings.ToLower(mac), ":", "")
	if len(h) < 6 {
		return ""
	}
	pfx := h[:6]
	switch {
	case strings.HasPrefix(h, "5254"):
		return "QEMU/KVM"
	case pfx == "005056", pfx == "000c29":
		return "VMware"
	case pfx == "00155d":
		return "Hyper-V"
	case pfx == "00163e":
		return "Xen"
	}
	if isLocallyAdministered(mac) {
		return "виртуализация (LAA)"
	}
	return ""
}

func appendUnique(xs []int, v int) []int {
	for _, x := range xs {
		if x == v {
			return xs
		}
	}
	return append(xs, v)
}

func buildHypotheses(rep *Report) []Hypothesis {
	var out []Hypothesis

	portMoves := 0
	devicePorts := map[int64]map[int]struct{}{}
	syslogMoves := 0
	for _, m := range rep.Timeline {
		if m.FromIfIndex != nil && m.ToIfIndex != nil {
			portMoves++
			if devicePorts[m.DeviceID] == nil {
				devicePorts[m.DeviceID] = map[int]struct{}{}
			}
			devicePorts[m.DeviceID][*m.FromIfIndex] = struct{}{}
			devicePorts[m.DeviceID][*m.ToIfIndex] = struct{}{}
		}
		if m.Source == store.MACMoveSourceSyslog {
			syslogMoves++
		}
	}

	accessHits := 0
	devicesAccess := map[int64]int{}
	for _, f := range rep.Footprint {
		if f.PortRole == "access" || f.PortRole == "" {
			accessHits++
			devicesAccess[f.DeviceID]++
		}
	}

	// core_loop_broadcast: MAC на ≥10 свитчах — бродкаст по ядру, не kvm_dual_uplink.
	kvmDualAdded := false
	devCount := distinctDevices(rep.Footprint)
	if devCount >= 10 {
		suspectAccess := []string{}
		for _, f := range rep.Footprint {
			if (f.PortRole == "access" || f.PortRole == "") && f.MACCount <= 5 {
				label := f.DeviceName
				if label == "" {
					label = strconv.FormatInt(f.DeviceID, 10)
				}
				suspectAccess = append(suspectAccess, label+" "+ifNameOrIndex(f))
			}
		}
		ev := []string{
			"MAC виден одновременно на " + strconv.Itoa(devCount) + " коммутаторах — бродкаст разлетелся по сети",
			"Это не «два uplinkа в один свитч», а петля в ядре сети (или STP выключен/слетел)",
		}
		if len(suspectAccess) > 0 {
			ev = append(ev, "Подозрительные access-порты (вероятный источник): "+strings.Join(suspectAccess, ", "))
		}
		out = append(out, Hypothesis{
			ID:         "core_loop_broadcast",
			Confidence: ConfidenceHigh,
			Title:      "Петля в ядре сети — MAC виден на множестве свитчей, бродкаст разлетелся",
			Evidence:   ev,
			SuggestedChecks: []string{
				"Проверить STP на ядре: show spanning-tree active",
				"Найти blocking-порт — если ни один не blocking, петля подтверждена",
				"Shut по очереди подозрительные access сверху footprint — flap прекратится на источнике",
				"События STP_TOPOLOGY_CHANGE / STP_ROOT_CHANGED в NetLynx",
				"spanning-tree + bpdu-guard на access-портах",
				"Сверить /investigate/loops",
			},
		})
		kvmDualAdded = true // не добавлять kvm_dual_uplink ниже
	}

	// dual-port flap на одном свитче без LLDP/CDP inventory
	for devID, ports := range devicePorts {
		if len(ports) < 2 {
			continue
		}
		portList := make([]int, 0, len(ports))
		for p := range ports {
			portList = append(portList, p)
		}
		var footOnDev []FootprintEntry
		for _, f := range rep.Footprint {
			if f.DeviceID == devID {
				footOnDev = append(footOnDev, f)
			}
		}
		noLLDP := true
		allAccessish := true
		for _, f := range footOnDev {
			if f.PortRole == "trunk" || f.PortRole == "ignore" {
				allAccessish = false
			}
			for _, n := range f.Neighbors {
				if n.Protocol == "lldp" || n.Protocol == "cdp" {
					noLLDP = false
				}
			}
		}
		if len(footOnDev) == 0 {
			noLLDP = true
			allAccessish = true
		}
		if !allAccessish || !noLLDP || (portMoves < 2 && syslogMoves < 1) {
			continue
		}

		portEv := formatPortList(portList, footOnDev)
		hv := hypervisorLabel(rep.Identity.MAC)

		if rep.Identity.VirtualizationHint && hv == "QEMU/KVM" {
			kvmDualAdded = true
			out = append(out, Hypothesis{
				ID:         "kvm_dual_uplink",
				Confidence: ConfidenceHigh,
				Title:      "QEMU/KVM: два uplink в один L2-домен (невидимое для LLDP кольцо)",
				Evidence: []string{
					"OUI 52:54:… — типичный MAC виртуальной машины QEMU/KVM",
					"MAC быстро перемещается между access-портами одного коммутатора: " + portEv,
					"На портах нет LLDP/CDP соседей — топология L2 такое кольцо не рисует",
					"Гипервизор или VM, вероятно, бриджит два кабеля к одному свитчу",
				},
				SuggestedChecks: []string{
					"Admin-down один из портов (" + portEv + ") — flapping должен прекратиться",
					"Найти физический хост за этими портами (KVM-гипервизор, Proxmox, libvirt)",
					"На гипервизоре: один uplink или bond+LACP; включить STP на Linux-bridge/OVS",
					"На свитче: spanning-tree + bpdu-guard на access-портах",
				},
			})
			break
		}

		out = append(out, Hypothesis{
			ID:         "unmanaged_loop",
			Confidence: ConfidenceHigh,
			Title:      "Петля через неуправляемое устройство или кабель между двумя портами",
			Evidence: []string{
				"MAC быстро перемещается между портами одного коммутатора",
				"Затронутые порты: " + portEv,
				"На затронутых портах нет LLDP/CDP соседей из inventory",
				"Топология L2 (LLDP) такое кольцо не рисует",
			},
			SuggestedChecks: []string{
				"Найти физические кабели на указанных портах (оба конца)",
				"Проверить, нет ли неуправляемого свитча/хаба, замыкающего эти порты",
				"Временно admin-down один из портов и проверить, прекратился ли flapping",
			},
		})
		break
	}

	// misclassified_uplink
	for _, f := range rep.Footprint {
		if (f.PortRole == "access" || f.PortRole == "") && f.MACCount >= 8 {
			out = append(out, Hypothesis{
				ID:         "misclassified_uplink",
				Confidence: ConfidenceMedium,
				Title:      "Порт похож на uplink/trunk, но роль access — возможны ложные MAC-события",
				Evidence: []string{
					"На порту много MAC в FDB (≥8)",
					"Роль порта: " + roleOrAccess(f.PortRole),
				},
				SuggestedChecks: []string{
					"Проверить switchport mode на коммутаторе (trunk vs access)",
					"Синхронизировать роли портов из конфига (SSH) или поправить роль вручную",
				},
			})
			break
		}
	}

	if rep.Identity.VirtualizationHint && !kvmDualAdded && (portMoves > 0 || syslogMoves > 0 || accessHits > 0) {
		hv := hypervisorLabel(rep.Identity.MAC)
		title := "MAC похож на виртуальный (гипервизор / LAA) — migrate, bridge или dual-homed VM"
		if hv != "" && hv != "виртуализация (LAA)" {
			title = hv + ": migrate, bridge или dual-homed VM"
		}
		ev := []string{vendorEvidence(rep.Identity)}
		if hv != "" {
			ev = append(ev, "Классификация по OUI/LAA: "+hv)
		}
		ev = append(ev, "Локально администрируемый и/или известный OUI виртуализации")
		out = append(out, Hypothesis{
			ID:              "virtualization_mac",
			Confidence:      ConfidenceMedium,
			Title:           title,
			Evidence:        ev,
			SuggestedChecks: virtualizationChecks(hv),
		})
	}

	if len(devicesAccess) >= 2 || accessHits >= 2 && distinctDevices(rep.Footprint) >= 2 {
		out = append(out, Hypothesis{
			ID:         "dual_homed_or_clone",
			Confidence: ConfidenceHigh,
			Title:      "Один MAC одновременно на access-портах разных коммутаторов",
			Evidence: []string{
				"Текущий FDB показывает MAC в нескольких местах access",
			},
			SuggestedChecks: []string{
				"Проверить spoofing / клон MAC",
				"Проверить L2-петлю через чужой сегмент",
				"Сверить ARP: один IP — один MAC на шлюзе",
			},
		})
	}

	for _, f := range rep.Footprint {
		if strings.EqualFold(f.Category, "ap") {
			out = append(out, Hypothesis{
				ID:         "ap_roaming",
				Confidence: ConfidenceLow,
				Title:      "MAC виден у точки доступа — возможен роуминг Wi‑Fi клиента",
				Evidence: []string{"Узел категории AP в footprint"},
				SuggestedChecks: []string{
					"Сверить с контроллером Wi‑Fi / логами роуминга",
					"Отличить клиентский MAC от BSSID точки",
				},
			})
			break
		}
		for _, n := range f.Neighbors {
			_ = n
		}
	}
	// AP via neighbor remote category is harder without resolve; skip for MVP beyond footprint category.

	if len(out) == 0 {
		conf := ConfidenceLow
		ev := []string{"Недостаточно истории перемещений или слабые сигналы"}
		if len(rep.Timeline) == 0 {
			ev = append(ev, "Нет записей в mac_fdb_moves — дождитесь опросов FDB или включите syslog")
		}
		out = append(out, Hypothesis{
			ID:         "insufficient_data",
			Confidence: conf,
			Title:      "Недостаточно данных для уверенного диагноза",
			Evidence:   ev,
			SuggestedChecks: []string{
				"Включить NETLYNX_SYSLOG_LISTEN и направить логи свитча (MAC_FLAPPING)",
				"Убедиться, что FDB-опрос на узле включён",
				"Повторить расследование после нескольких циклов FDB",
			},
		})
	}
	return out
}

func roleOrAccess(r string) string {
	if r == "" {
		return "access (по умолчанию)"
	}
	return r
}

func vendorEvidence(id Identity) string {
	if id.Vendor != "" {
		return "Vendor/OUI: " + id.Vendor
	}
	if id.VirtualizationHint {
		return "Префикс MAC типичен для виртуализации (например 52:54:…)"
	}
	return "Vendor неизвестен"
}

func distinctDevices(foot []FootprintEntry) int {
	m := map[int64]struct{}{}
	for _, f := range foot {
		m[f.DeviceID] = struct{}{}
	}
	return len(m)
}

// sortFootprint: access с малым MACCount сверху (вероятный источник), trunk внизу.
func sortFootprint(foot []FootprintEntry) {
	sort.SliceStable(foot, func(i, j int) bool {
		a, b := foot[i], foot[j]
		ra, rb := accessRank(a.PortRole), accessRank(b.PortRole)
		if ra != rb {
			return ra < rb
		}
		susA := (a.PortRole == "access" || a.PortRole == "") && a.MACCount <= 5
		susB := (b.PortRole == "access" || b.PortRole == "") && b.MACCount <= 5
		if susA != susB {
			return susA && !susB
		}
		if a.MACCount != b.MACCount {
			return a.MACCount < b.MACCount
		}
		return a.DeviceName < b.DeviceName
	})
}

func accessRank(role string) int {
	switch role {
	case "access", "":
		return 0
	case "trunk":
		return 2
	case "ignore":
		return 3
	default:
		return 4
	}
}

func ifNameOrIndex(f FootprintEntry) string {
	if f.IfName != nil && *f.IfName != "" {
		return *f.IfName
	}
	if f.IfDescr != nil && *f.IfDescr != "" {
		return *f.IfDescr
	}
	return "if" + strconv.Itoa(f.IfIndex)
}

func formatPortList(portIndexes []int, foot []FootprintEntry) string {
	nameByIdx := map[int]string{}
	for _, f := range foot {
		if f.IfName != nil && *f.IfName != "" {
			nameByIdx[f.IfIndex] = *f.IfName
		} else if f.IfDescr != nil && *f.IfDescr != "" {
			nameByIdx[f.IfIndex] = *f.IfDescr
		}
	}
	var parts []string
	for _, p := range portIndexes {
		if n := nameByIdx[p]; n != "" {
			parts = append(parts, n+" (ifIndex "+strconv.Itoa(p)+")")
		} else {
			parts = append(parts, "ifIndex "+strconv.Itoa(p))
		}
	}
	return strings.Join(parts, ", ")
}

func virtualizationChecks(hv string) []string {
	base := []string{
		"Найти VM/контейнер с этим MAC на гипервизорах",
		"Проверить bridging/bonding с одним MAC на двух uplink",
		"Исключить клон NIC / шаблон с фиксированным MAC",
	}
	if hv == "QEMU/KVM" {
		return append([]string{
			"На libvirt/Proxmox: не бриджить два физических интерфейса в один linux-bridge без STP",
		}, base...)
	}
	return base
}
