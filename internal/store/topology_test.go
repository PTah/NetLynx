package store

import (
	"testing"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/models"
)

func TestExpandNameKeys(t *testing.T) {
	got := expandNameKeys("SW-Core.example.com")
	if len(got) < 2 || got[0] != "sw-core.example.com" || got[1] != "sw-core" {
		t.Fatalf("unexpected keys: %#v", got)
	}
	ipKeys := expandNameKeys("192.168.160.54")
	if len(ipKeys) != 1 || ipKeys[0] != "192.168.160.54" {
		t.Fatalf("IP must not strip first octet: %#v", ipKeys)
	}
	ipKeys = expandNameKeys("48.47.50.52")
	if len(ipKeys) != 1 || ipKeys[0] != "48.47.50.52" {
		t.Fatalf("IP-like must stay whole: %#v", ipKeys)
	}
}

func TestResolveRemoteDeviceID(t *testing.T) {
	sys := "sw-access-1.lan"
	devices := []models.Device{
		{ID: 10, Name: "Access-1", Host: "10.0.0.5", SysName: &sys},
		{ID: 20, Name: "Core", Host: "10.0.0.1"},
	}
	idx := buildDeviceNameIndex(devices)

	name := "SW-ACCESS-1"
	id, ok := resolveRemoteDeviceID(idx, PortNeighbor{RemoteSysName: &name})
	if !ok || id != 10 {
		t.Fatalf("want device 10, got ok=%v id=%d", ok, id)
	}

	hostish := "10.0.0.1"
	id, ok = resolveRemoteDeviceID(idx, PortNeighbor{RemoteMgmtAddr: &hostish})
	if !ok || id != 20 {
		t.Fatalf("want device 20 by host, got ok=%v id=%d", ok, id)
	}
}

func TestResolveRemoteByChassisMAC(t *testing.T) {
	mac := "f0:9f:c2:64:fe:5c"
	devices := []models.Device{
		{ID: 70, Name: "ES48-4", Host: "192.168.160.21", ChassisMAC: &mac},
		{ID: 36, Name: "ES24-2", Host: "192.168.160.64"},
	}
	idx := buildDeviceNameIndex(devices)
	ch := "F0:9F:C2:64:FE:5C"
	sys := "EdgeSwitch" // ambiguous / unused
	id, ok := resolveRemoteDeviceID(idx, PortNeighbor{RemoteSysName: &sys, RemoteChassisID: &ch})
	if !ok || id != 70 {
		t.Fatalf("want device 70 by chassis MAC, got ok=%v id=%d", ok, id)
	}
}

func TestResolveRemotePrefersIPOverSharedSysName(t *testing.T) {
	sys := "EdgeSwitch"
	devices := []models.Device{
		{ID: 1, Name: "ES-A", Host: "10.0.0.1", SysName: &sys},
		{ID: 2, Name: "ES-B", Host: "10.0.0.2", SysName: &sys},
	}
	idx := buildDeviceNameIndex(devices)
	name := "EdgeSwitch"
	if _, ok := resolveRemoteDeviceID(idx, PortNeighbor{RemoteSysName: &name}); ok {
		t.Fatal("ambiguous model sysName must not resolve")
	}
	mgmt := "10.0.0.2"
	id, ok := resolveRemoteDeviceID(idx, PortNeighbor{RemoteSysName: &name, RemoteMgmtAddr: &mgmt})
	if !ok || id != 2 {
		t.Fatalf("want device 2 by IP despite shared sysName, got ok=%v id=%d", ok, id)
	}
}

func TestClassifyTopologyKind(t *testing.T) {
	if got := classifyTopologyKind(true, "", "", "", "aa:bb:cc:dd:ee:ff"); got != "virtual" {
		t.Fatalf("virtual: %s", got)
	}
	if got := classifyTopologyKind(false, "server", "", "", "HP-Hyper-V"); got != "server" {
		t.Fatalf("server: %s", got)
	}
	if got := classifyTopologyKind(false, "mfu", "", "", "Xerox"); got != "mfu" {
		t.Fatalf("mfu: %s", got)
	}
	if got := classifyTopologyKind(false, "camera", "", "", "cam1"); got != "camera" {
		t.Fatalf("camera: %s", got)
	}
	if got := classifyTopologyKind(false, "tv", "", "Samsung", "Samsung TV"); got != "tv" {
		t.Fatalf("custom tv: %s", got)
	}
	if got := classifyTopologyKind(false, "rack", "", "", "Шкаф"); got != "rack" {
		t.Fatalf("custom rack: %s", got)
	}
	if got := classifyTopologyKind(false, "", "", "Yealink SIP-T41S", "SIP-T41S"); got != "phone" {
		t.Fatalf("phone: %s", got)
	}
	if got := classifyTopologyKind(false, "", "ubiquiti", "EdgeSwitch 24", "sw1"); got != "switch" {
		t.Fatalf("switch: %s", got)
	}
	if got := classifyTopologyKind(false, "", "", "UniFi AP AC Pro", "ap-hall"); got != "ap" {
		t.Fatalf("ap: %s", got)
	}
	if got := classifyTopologyKind(false, "ap", "", "", "office-ap"); got != "ap" {
		t.Fatalf("ap category: %s", got)
	}
	if got := classifyTopologyKind(false, "", "", "RouterOS CCR1009", "CCR1009-SkyNet"); got != "router" {
		t.Fatalf("ccr router: %s", got)
	}
	if got := classifyTopologyKind(false, "", "", "MikroTik CRS326", "crs-core"); got != "switch" {
		t.Fatalf("crs switch: %s", got)
	}
	if got := classifyTopologyKind(false, "", "", "Hikvision DS-2CD2143G0-I", "cam-yard"); got != "camera" {
		t.Fatalf("hik camera: %s", got)
	}
	if got := classifyTopologyKind(false, "", "", "Hikvision DS-3E1526P-EI Switch", "hik-sw"); got != "switch" {
		t.Fatalf("hik switch: %s", got)
	}
	if got := classifyTopologyKind(false, "", "", "Dahua DH-PFS4218-16ET-190", "pfs"); got != "switch" {
		t.Fatalf("dahua switch: %s", got)
	}
}

func TestMergeTopologyEdgeMetaStaleFreshWins(t *testing.T) {
	stale := TopologyEdge{Stale: true}
	fresh := TopologyEdge{Stale: false}
	mergeTopologyEdgeMeta(&stale, fresh)
	if stale.Stale {
		t.Fatal("fresh neighbor must clear stale on merged edge")
	}
	live := TopologyEdge{Stale: false}
	ghost := TopologyEdge{Stale: true}
	mergeTopologyEdgeMeta(&live, ghost)
	if live.Stale {
		t.Fatal("stale ghost must not poison live edge")
	}
	onlyStale := TopologyEdge{Stale: true}
	mergeTopologyEdgeMeta(&onlyStale, TopologyEdge{Stale: true})
	if !onlyStale.Stale {
		t.Fatal("all-stale merge should stay stale")
	}
}
