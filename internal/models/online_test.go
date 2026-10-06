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

func TestDeviceIsOnlinePingMode(t *testing.T) {
	pingOK := true
	pingBad := false
	snmpOK := true

	// IoT / «switch» без SNMP: ping-mode смотрит только на ICMP.
	d := Device{DeviceCategory: "switch", ReachabilityMode: "ping", LastPingOK: &pingOK, LastSNMPOK: nil}
	if !d.IsOnline() {
		t.Fatal("ping mode: ping ok => online even for switch")
	}
	d = Device{DeviceCategory: "other", ReachabilityMode: "ping", LastPingOK: &pingBad, LastSNMPOK: &snmpOK}
	if d.IsOnline() {
		t.Fatal("ping mode: ignore SNMP, ping fail => offline")
	}
	d = Device{ReachabilityMode: "ping"}
	if d.IsOnline() {
		t.Fatal("ping mode: never polled => offline")
	}
}

func TestNormalizeReachabilityMode(t *testing.T) {
	on := true
	off := false
	if NormalizeReachabilityMode("", nil) != "auto" {
		t.Fatal("empty")
	}
	if NormalizeReachabilityMode("PING", nil) != "ping" {
		t.Fatal("ping")
	}
	if NormalizeReachabilityMode("", &on) != "online" {
		t.Fatal("legacy online")
	}
	if NormalizeReachabilityMode("", &off) != "offline" {
		t.Fatal("legacy offline")
	}
}
