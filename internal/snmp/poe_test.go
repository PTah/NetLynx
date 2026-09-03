package snmp

import "testing"

func TestGuessIfIndexPrefersEdgeSwitchPhysical(t *testing.T) {
	rows := map[int]IfRow{
		5:  {IfName: "Vl1", IfType: 53, Oper: 1},
		13: {IfName: "0/5", IfType: 6, Oper: 1},
		49: {IfName: "1/5", IfType: 6, Oper: 1},
	}
	got := guessIfIndexFromPsePortIndex(rows, 5)
	if got != 13 {
		t.Fatalf("got ifIndex %d, want 13 (0/5 ethernet)", got)
	}
}

func TestParseBroadcomSuffixPrefersEthernetOverVLAN(t *testing.T) {
	rows := map[int]IfRow{
		5:  {IfName: "Vl5", IfType: 53, Oper: 1},
		13: {IfName: "0/5", IfType: 6, Oper: 1},
	}
	got := parseIfIndexFromBroadcomPethSuffix("5.13", rows)
	if got != 13 {
		t.Fatalf("got ifIndex %d, want 13 (ethernet over VLAN)", got)
	}
}

func TestPethDeliveringConstant(t *testing.T) {
	if pethDeliveringPower != 3 {
		t.Fatalf("pethDeliveringPower=%d", pethDeliveringPower)
	}
}
