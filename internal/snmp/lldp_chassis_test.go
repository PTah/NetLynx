package snmp

import "testing"

func TestApplyLLDPChassisNetworkAddress(t *testing.T) {
	var n NeighborInfo
	// family=1 + 192.168.170.73
	applyLLDPChassis(&n, lldpChassisSubtypeNetwork, []byte{1, 192, 168, 170, 73})
	if n.RemoteMgmtAddr != "192.168.170.73" {
		t.Fatalf("mgmt=%q", n.RemoteMgmtAddr)
	}
	if n.RemoteChassisID != "" {
		t.Fatalf("chassis should stay empty, got %q", n.RemoteChassisID)
	}
}

func TestApplyLLDPChassisMAC(t *testing.T) {
	var n NeighborInfo
	applyLLDPChassis(&n, lldpChassisSubtypeMAC, []byte{0x0c, 0x38, 0x3e, 0x5a, 0xf1, 0x56})
	if n.RemoteChassisID != "0c:38:3e:5a:f1:56" {
		t.Fatalf("chassis=%q", n.RemoteChassisID)
	}
}

func TestDecodeNetworkAddressHex(t *testing.T) {
	if got := decodeNetworkAddressHex("01c0a8aa49"); got != "192.168.170.73" {
		t.Fatalf("got %q", got)
	}
	if got := decodeNetworkAddressHex("01:c0:a8:aa:49"); got != "192.168.170.73" {
		t.Fatalf("colon got %q", got)
	}
	if got := decodeNetworkAddressHex("0c383e5af156"); got != "" {
		t.Fatalf("real mac must not decode as IP, got %q", got)
	}
}
