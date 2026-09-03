package store

import (
	"testing"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/models"
)

func TestDiscoveredIdentityKey(t *testing.T) {
	tests := []struct {
		name    string
		sys     string
		mgmt    string
		chassis string
		port    string
		want    string
	}{
		{"sysname", "SW-A.lan", "", "", "", "name:sw-a.lan"},
		{"mgmt over model name", "SIP-T41S", "192.168.170.29", "", "", "addr:192.168.170.29"},
		{"chassis over model name", "SIP-T41S", "", "00:15:65:c7:6e:7f", "", "chassis:001565c76e7f"},
		{"port mac as chassis", "SIP-T41S", "", "", "00:15:65:c7:6e:7f", "chassis:001565c76e7f"},
		{"mgmt fallback", "", "10.1.2.3", "aa:bb", "", "addr:10.1.2.3"},
		{"chassis short", "", "", "AA:BB:CC", "", "chassis:aabbcc"},
		{"chassis full colon", "", "", "34:5A:60:F2:E6:5A", "", "chassis:345a60f2e65a"},
		{"chassis full compact", "", "", "345a60f2e65a", "", "chassis:345a60f2e65a"},
		{"empty", "", "", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DiscoveredIdentityKeyWithPort(tt.sys, tt.mgmt, tt.chassis, tt.port)
			if got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestDiscoveredChassisMAC(t *testing.T) {
	ch := "B4:7A:F1:DD:C4:44"
	d := &DiscoveredDevice{IdentityKey: "chassis:b47af1ddc444", RemoteChassisID: &ch}
	if got := DiscoveredChassisMAC(d); got != "b4:7a:f1:dd:c4:44" {
		t.Fatalf("from remote_chassis_id: %q", got)
	}
	d2 := &DiscoveredDevice{IdentityKey: "chassis:b47af1ddc444"}
	if got := DiscoveredChassisMAC(d2); got != "b4:7a:f1:dd:c4:44" {
		t.Fatalf("from identity_key: %q", got)
	}
	if DiscoveredChassisMAC(nil) != "" {
		t.Fatal("nil")
	}
}

func TestVirtualPeerIdentity(t *testing.T) {
	sys := "SIP-T41S"
	mgmt := "192.168.170.29"
	port := "00:15:65:c7:6e:7f"
	e := TopologyEdge{RemoteSysName: &sys, RemoteMgmtAddr: &mgmt, RemotePortID: &port}
	key, label := virtualPeerIdentity(e)
	if key != "chassis:001565c76e7f" {
		t.Fatalf("port MAC should win: key=%q label=%q", key, label)
	}
	if label != "SIP-T41S · 192.168.170.29" {
		t.Fatalf("label: %q", label)
	}
	e2 := TopologyEdge{RemoteSysName: &sys, RemoteMgmtAddr: &mgmt}
	key2, label2 := virtualPeerIdentity(e2)
	if key2 != "addr:192.168.170.29" {
		t.Fatalf("mgmt key: %q", key2)
	}
	if label2 != "SIP-T41S · 192.168.170.29" {
		t.Fatalf("label: %q", label2)
	}
	// Without mgmt/MAC — last resort model name (may still collapse identical models).
	e3 := TopologyEdge{RemoteSysName: &sys}
	key3, _ := virtualPeerIdentity(e3)
	if key3 != "name:sip-t41s" {
		t.Fatalf("sysname-only key: %q", key3)
	}
}

func TestShouldOfferDiscovered(t *testing.T) {
	sys := "known-sw"
	devices := []models.Device{
		{ID: 1, Name: "Known", Host: "10.0.0.1", SysName: &sys},
	}
	idx := buildDeviceNameIndex(devices)

	knownName := "KNOWN-SW.example"
	nbKnown := PortNeighbor{RemoteSysName: &knownName, IfIndex: 10}
	if _, ok := ShouldOfferDiscovered(idx, nbKnown); ok {
		t.Fatal("known neighbor must not be offered")
	}

	unknown := "new-leaf-99"
	nbNew := PortNeighbor{RemoteSysName: &unknown, IfIndex: 12, Protocol: "lldp"}
	id, ok := ShouldOfferDiscovered(idx, nbNew)
	if !ok || id != "name:new-leaf-99" {
		t.Fatalf("want offer name:new-leaf-99, got ok=%v id=%q", ok, id)
	}
}

func TestHideDiscoveredAlreadyInInventory(t *testing.T) {
	mac := "00:15:65:c7:6e:7f"
	ip := "192.168.170.29"
	devices := []models.Device{
		{ID: 10, Name: "Phone", Host: ip, ChassisMAC: &mac},
		{ID: 11, Name: "Core", Host: "10.0.0.1"},
	}
	otherMAC := "aa:bb:cc:dd:ee:ff"
	otherIP := "10.9.8.7"
	list := []DiscoveredDevice{
		{ID: 1, Status: DiscoveredStatusNew, IdentityKey: "chassis:001565c76e7f", RemoteChassisID: &mac},
		{ID: 2, Status: DiscoveredStatusNew, IdentityKey: "addr:192.168.170.29", RemoteMgmtAddr: &ip},
		{ID: 3, Status: DiscoveredStatusNew, IdentityKey: "chassis:aabbccddeeff", RemoteChassisID: &otherMAC},
		{ID: 4, Status: DiscoveredStatusNew, IdentityKey: "addr:10.9.8.7", RemoteMgmtAddr: &otherIP},
		{ID: 5, Status: DiscoveredStatusAdded, IdentityKey: "chassis:001565c76e7f", RemoteChassisID: &mac, PromotedDeviceID: int64Ptr(10)},
	}
	got := hideDiscoveredAlreadyInInventory(list, devices)
	if len(got) != 3 {
		t.Fatalf("want 3 remaining, got %d: %+v", len(got), got)
	}
	ids := map[int64]bool{}
	for _, d := range got {
		ids[d.ID] = true
	}
	if ids[1] || ids[2] {
		t.Fatalf("known MAC/IP must be hidden from new: %+v", got)
	}
	if !ids[3] || !ids[4] || !ids[5] {
		t.Fatalf("unknown new and added must remain: %+v", got)
	}
}

func int64Ptr(v int64) *int64 { return &v }

func TestFilterUnknownNeighbors(t *testing.T) {
	sys := "core1"
	devices := []models.Device{{ID: 1, Name: "Core", Host: "1.1.1.1", SysName: &sys}}
	a := "core1"
	b := "leaf-x"
	neighbors := []PortNeighbor{
		{RemoteSysName: &a, IfIndex: 1},
		{RemoteSysName: &b, IfIndex: 2},
	}
	got := FilterUnknownNeighbors(devices, neighbors)
	if len(got) != 1 || derefStr(got[0].RemoteSysName) != "leaf-x" {
		t.Fatalf("unexpected filter result: %+v", got)
	}
}

func TestSuggestDiscoveredHostAndName(t *testing.T) {
	mgmt := "192.168.1.50"
	sys := "leaf-50"
	d := &DiscoveredDevice{RemoteMgmtAddr: &mgmt, RemoteSysName: &sys, IdentityKey: "name:leaf-50"}
	if SuggestDiscoveredHost(d) != "192.168.1.50" {
		t.Fatalf("host: %q", SuggestDiscoveredHost(d))
	}
	if SuggestDiscoveredName(d) != "leaf-50" {
		t.Fatalf("name: %q", SuggestDiscoveredName(d))
	}
}
