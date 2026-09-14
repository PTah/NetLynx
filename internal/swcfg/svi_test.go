package swcfg

import "testing"

func TestVLANIDFromSVIIfaceName(t *testing.T) {
	cases := map[string]int{
		"Vlan100":             100,
		"vlan 14":             14,
		"Vlanif200":           200,
		"Vlan-interface30":    30,
		"GigabitEthernet0/1":  0,
	}
	for name, want := range cases {
		got, ok := VLANIDFromSVIIfaceName(name)
		if want == 0 {
			if ok {
				t.Fatalf("%s: expected fail, got %d", name, got)
			}
			continue
		}
		if !ok || got != want {
			t.Fatalf("%s: got %d ok=%v want %d", name, got, ok, want)
		}
	}
}

func TestParseSVIAddresses(t *testing.T) {
	cfg := `
!
interface GigabitEthernet0/1
 switchport mode access
!
interface Vlan1
 ip address 192.168.1.10 255.255.255.0
!
interface Vlan100
 description mgmt
 ip address 10.0.0.1/24
!
interface Vlanif50
 ip address 172.16.0.1 255.255.255.0
!
`
	svis := ParseSVIAddresses(cfg)
	byVLAN := map[int]string{}
	for _, s := range svis {
		byVLAN[s.VLANID] = s.IP
	}
	if byVLAN[1] != "192.168.1.10" || byVLAN[100] != "10.0.0.1" || byVLAN[50] != "172.16.0.1" {
		t.Fatalf("got %+v", svis)
	}
}

func TestMgmtVLANFromSVI(t *testing.T) {
	svis := []SVIAddress{
		{VLANID: 1, IP: "192.168.1.1"},
		{VLANID: 99, IP: "10.10.10.5"},
	}
	vid, ip, ok := MgmtVLANFromSVI("10.10.10.5", svis)
	if !ok || vid != 99 || ip != "10.10.10.5" {
		t.Fatalf("got %d %s %v", vid, ip, ok)
	}
	if _, _, ok := MgmtVLANFromSVI("10.10.10.6", svis); ok {
		t.Fatal("no match")
	}
	if _, _, ok := MgmtVLANFromSVI("switch.local", svis); ok {
		t.Fatal("hostname not IP")
	}
}
