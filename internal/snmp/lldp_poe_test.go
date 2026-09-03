package snmp

import "testing"

func TestFilterLldpPdInfraUplinksEltexUplink(t *testing.T) {
	pd := map[int]bool{1: true, 2: true}
	neighbors := []NeighborInfo{
		{IfIndex: 1, RemoteSysName: "Eltex 2324 Upper", RemotePortID: "gi1/0/23"},
		{IfIndex: 2, RemoteSysName: "UAP-AC-Pro", RemotePortID: "eth0"},
	}
	FilterLldpPdInfraUplinks(pd, neighbors)
	if pd[1] {
		t.Fatal("port 1 uplink to switch should not be PoE")
	}
	if !pd[2] {
		t.Fatal("port 2 AP should remain PoE candidate")
	}
}

func TestLooksLikeInfraNeighborGiPort(t *testing.T) {
	if !looksLikeInfraNeighbor(NeighborInfo{RemotePortID: "gi1/0/23"}) {
		t.Fatal("gi port should be infra")
	}
	if !looksLikeInfraNeighbor(NeighborInfo{RemotePortID: "eth1/0/1"}) {
		t.Fatal("eth with slash should be infra")
	}
	if looksLikeInfraNeighbor(NeighborInfo{RemotePortID: "eth0", RemoteSysName: "camera-1"}) {
		t.Fatal("eth0 camera should not be infra")
	}
}
