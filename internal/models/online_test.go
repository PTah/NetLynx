package models

import "testing"

func TestDeviceIsOnlineSwitch(t *testing.T) {
	snmpOK := true
	snmpBad := false
	pingOK := true
	off := false
	on := true

	d := Device{DeviceCategory: "switch", LastSNMPOK: &snmpOK}
	if !d.IsOnline() {
		t.Fatal("snmp ok")
	}
	d = Device{DeviceCategory: "switch", LastPingOK: &pingOK, LastSNMPOK: &snmpBad}
	if d.IsOnline() {
		t.Fatal("switch: ping without snmp is offline")
	}
	d = Device{DeviceCategory: "switch"}
	if d.IsOnline() {
		t.Fatal("never polled is offline")
	}
	d = Device{DeviceCategory: "switch", OnlineOverride: &on}
	if !d.IsOnline() {
		t.Fatal("manual online")
	}
	d = Device{DeviceCategory: "switch", LastSNMPOK: &snmpOK, OnlineOverride: &off}
	if d.IsOnline() {
		t.Fatal("manual offline")
	}
}
