package swcfg

import "testing"

func TestIsMikrotikRouterDevice(t *testing.T) {
	if !IsMikrotikRouterDevice("router", "auto", "RouterOS 7.12 on RB2011", "RB2011-GATE") {
		t.Fatal("RB2011 router")
	}
	if !IsMikrotikRouterDevice("router", "mikrotik", "", "CCR1009") {
		t.Fatal("explicit mikrotik vendor")
	}
	if IsMikrotikRouterDevice("switch", "auto", "RouterOS CRS326", "crs-core") {
		t.Fatal("CRS switch is not mikrotik router policy")
	}
	if IsMikrotikRouterDevice("router", "auto", "Cisco IOS", "ISR4331") {
		t.Fatal("cisco router")
	}
}

func TestIsMikrotikRouterForConfigBackup(t *testing.T) {
	if !IsMikrotikRouterForConfigBackup("router", "mikrotik") {
		t.Fatal("explicit mikrotik")
	}
	if IsMikrotikRouterForConfigBackup("router", "auto") {
		t.Fatal("auto vendor")
	}
	if IsMikrotikRouterForConfigBackup("router", "") {
		t.Fatal("empty vendor")
	}
	if IsMikrotikRouterForConfigBackup("switch", "mikrotik") {
		t.Fatal("switch")
	}
}
