package api

import (
	"strings"
	"testing"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

func TestPollDeviceFromDiscoveredBody(t *testing.T) {
	mgmt := "10.9.8.7"
	sys := "sw-cand"
	d := &store.DiscoveredDevice{RemoteMgmtAddr: &mgmt, RemoteSysName: &sys}
	comm := "public"

	pd, errMsg := pollDeviceFromDiscoveredBody(d, discoveredSNMPBody{
		SNMPVersion: "v2c",
		Community:   &comm,
	}, true)
	if errMsg != "" {
		t.Fatalf("unexpected err: %s", errMsg)
	}
	if pd.Host != "10.9.8.7" || pd.SNMPVersion != "v2c" {
		t.Fatalf("unexpected poll device: %+v", pd)
	}

	_, errMsg = pollDeviceFromDiscoveredBody(&store.DiscoveredDevice{}, discoveredSNMPBody{
		SNMPVersion: "v2c",
		Community:   &comm,
	}, true)
	if errMsg == "" {
		t.Fatal("expected host required error")
	}

	_, errMsg = pollDeviceFromDiscoveredBody(d, discoveredSNMPBody{SNMPVersion: "v2c"}, true)
	if errMsg == "" {
		t.Fatal("expected community required")
	}
}

func TestNormalizePortClientMAC(t *testing.T) {
	mac, errMsg := normalizePortClientMAC("AA-BB-CC-DD-EE-FF")
	if errMsg != "" {
		t.Fatalf("unexpected err: %s", errMsg)
	}
	if mac != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("got %q", mac)
	}
	if _, errMsg := normalizePortClientMAC("aa:bb"); errMsg == "" {
		t.Fatal("expected short MAC error")
	}
	if _, errMsg := normalizePortClientMAC(""); errMsg == "" {
		t.Fatal("expected empty MAC error")
	}
}

func TestSyntheticDiscoveredFromClient(t *testing.T) {
	d := syntheticDiscoveredFromClient("aa:bb:cc:dd:ee:ff", "10.1.2.3")
	if d.RemoteChassisID == nil || *d.RemoteChassisID != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("chassis: %+v", d.RemoteChassisID)
	}
	if d.RemoteMgmtAddr == nil || *d.RemoteMgmtAddr != "10.1.2.3" {
		t.Fatalf("mgmt: %+v", d.RemoteMgmtAddr)
	}
	comm := "public"
	pd, errMsg := pollDeviceFromDiscoveredBody(d, discoveredSNMPBody{SNMPVersion: "v2c", Community: &comm}, false)
	if errMsg != "" {
		t.Fatal(errMsg)
	}
	if pd.Host != "10.1.2.3" {
		t.Fatalf("host %q", pd.Host)
	}

	loop := "127.0.0.1"
	dLoop := &store.DiscoveredDevice{RemoteMgmtAddr: &loop}
	_, errMsg = pollDeviceFromDiscoveredBody(dLoop, discoveredSNMPBody{SNMPVersion: "v2c", Community: &comm}, true)
	if errMsg == "" || !strings.Contains(errMsg, "loopback") {
		t.Fatalf("expected loopback reject, got %q", errMsg)
	}
}
