package snmp

import "testing"

func TestUbiquitiLikelySFPPort_ES24(t *testing.T) {
	sys := "EdgeSwitch 24 L26 #1, Linux 3.6.5"
	if !UbiquitiLikelySFPPort(sys, 26) || !UbiquitiLikelySFPPort(sys, 25) {
		t.Fatal("25/26 must be SFP on ES24")
	}
	if UbiquitiLikelySFPPort(sys, 24) || UbiquitiLikelySFPPort(sys, 1) {
		t.Fatal("copper must not be SFP")
	}
}

func TestClearPoEOnFiberPorts(t *testing.T) {
	sys := "EdgeSwitch 24-Port 250W"
	ifRows := map[int]IfRow{
		20: {IfIndex: 20, IfName: "0/20"},
		26: {IfIndex: 26, IfName: "0/26"},
	}
	poe := map[int]bool{20: true, 26: true}
	ClearPoEOnFiberPorts(poe, ifRows, sys)
	if !poe[20] {
		t.Fatal("copper PoE must stay")
	}
	if poe[26] {
		t.Fatal("SFP must clear PoE")
	}
}
