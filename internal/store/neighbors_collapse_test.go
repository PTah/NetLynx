package store

import (
	"testing"
	"time"
)

func sp(s string) *string { return &s }

func TestCollapsePortNeighborsByIdentity(t *testing.T) {
	t0 := time.Date(2026, 7, 31, 1, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Minute)
	in := []PortNeighbor{
		{DeviceID: 50, IfIndex: 12, RemIndex: 100, Protocol: "lldp", RemoteSysName: sp("CCR"), RemotePortID: sp("VLAN-10"), Stale: true, LastSeenAt: t0},
		{DeviceID: 50, IfIndex: 12, RemIndex: 200, Protocol: "lldp", RemoteSysName: sp("CCR"), RemotePortID: sp("VLAN-10"), Stale: true, LastSeenAt: t0.Add(30 * time.Second)},
		{DeviceID: 50, IfIndex: 12, RemIndex: 300, Protocol: "lldp", RemoteSysName: sp("CCR"), RemotePortID: sp("VLAN-10"), Stale: false, LastSeenAt: t1},
		{DeviceID: 50, IfIndex: 11, RemIndex: 1, Protocol: "lldp", RemoteSysName: sp("ES1"), RemotePortID: sp("0/12"), Stale: false, LastSeenAt: t1},
		{DeviceID: 50, IfIndex: 12, RemIndex: 1, Protocol: "cdp", RemoteSysName: sp("CCR"), RemotePortID: sp("VLAN-10"), Stale: false, LastSeenAt: t1},
	}
	out := CollapsePortNeighborsByIdentity(in)
	if len(out) != 3 {
		t.Fatalf("want 3 collapsed, got %d", len(out))
	}
	var liveLLDP *PortNeighbor
	for i := range out {
		n := &out[i]
		if n.IfIndex == 12 && n.Protocol == "lldp" {
			liveLLDP = n
		}
	}
	if liveLLDP == nil {
		t.Fatal("missing collapsed LLDP on if 12")
	}
	if liveLLDP.Stale || liveLLDP.RemIndex != 300 {
		t.Fatalf("want live rem_index=300, got stale=%v rem=%d", liveLLDP.Stale, liveLLDP.RemIndex)
	}
}

func TestCollapsePortNeighborsKeepsFreshestStale(t *testing.T) {
	t0 := time.Date(2026, 7, 31, 1, 0, 0, 0, time.UTC)
	in := []PortNeighbor{
		{DeviceID: 1, IfIndex: 1, RemIndex: 10, Protocol: "lldp", RemoteSysName: sp("A"), Stale: true, LastSeenAt: t0},
		{DeviceID: 1, IfIndex: 1, RemIndex: 20, Protocol: "lldp", RemoteSysName: sp("A"), Stale: true, LastSeenAt: t0.Add(time.Hour)},
	}
	out := CollapsePortNeighborsByIdentity(in)
	if len(out) != 1 || out[0].RemIndex != 20 {
		t.Fatalf("want freshest stale rem=20, got %+v", out)
	}
}
