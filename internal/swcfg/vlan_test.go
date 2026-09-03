package swcfg

import (
	"strings"
	"testing"
)

func TestParseVLANDatabase_EdgeSwitch(t *testing.T) {
	raw := `
vlan database
vlan 1,10,20,100-102
vlan name 10 Office
vlan name 20 "Cameras"
exit
interface 0/2
vlan pvid 10
switchport mode access
switchport access vlan 10
exit
`
	names := ParseVLANDatabase(raw)
	if names[10] != "Office" || names[20] != "Cameras" {
		t.Fatalf("names: %+v", names)
	}
	if _, ok := names[100]; !ok {
		t.Fatal("missing 100")
	}
	if _, ok := names[102]; !ok {
		t.Fatal("missing 102")
	}
}

func TestParseVLANDatabase_EltexOneLiner(t *testing.T) {
	raw := `
vlan database
vlan 10 name Management
vlan 20 name "Video Cameras"
exit
vlan 30 name Office
`
	names := ParseVLANDatabase(raw)
	if names[10] != "Management" || names[20] != "Video Cameras" || names[30] != "Office" {
		t.Fatalf("%+v", names)
	}
}

func TestParseVLANDatabase_CiscoIOS(t *testing.T) {
	raw := `
vlan 10
 name Voice
vlan 30
 name Cameras
interface GigabitEthernet0/1
 switchport access vlan 10
`
	names := ParseVLANDatabase(raw)
	if names[10] != "Voice" || names[30] != "Cameras" {
		t.Fatalf("%+v", names)
	}
}

func TestBuildVLANInventory_staleAccessIgnoredWhenConfigHasPort(t *testing.T) {
	cfg := `
vlan database
vlan 1,10
exit
interface 0/2
switchport mode access
switchport access vlan 10
exit
`
	stale := 167
	ports := []PortVLANHint{
		{IfIndex: 2, IfName: "0/2", Role: "access", AccessVLAN: &stale},
	}
	inv := BuildVLANInventory(cfg, ports, nil)
	ids := map[int]bool{}
	for _, r := range inv {
		ids[r.VLANID] = true
		if r.VLANID == 10 && !r.InDatabase {
			t.Fatal("10 should be in database")
		}
	}
	if ids[167] {
		t.Fatal("stale cli_access_vlan 167 must not invent a row when show run has the port")
	}
}

func TestBuildVLANInventory_fdbOnlyNotInvented(t *testing.T) {
	cfg := `
vlan database
vlan 1,10
exit
`
	inv := BuildVLANInventory(cfg, nil, []FDBVLANPort{
		{IfIndex: 3, VLANID: 167, IfName: "0/3"},
		{IfIndex: 3, VLANID: 10, IfName: "0/3"},
	})
	byID := map[int]VLANInventoryRow{}
	for _, r := range inv {
		byID[r.VLANID] = r
	}
	if _, ok := byID[167]; ok {
		t.Fatal("FDB-only VLAN must not invent a row after vlan deleted from switch")
	}
	if len(byID[10].FDBPorts) != 1 || byID[10].FDBPorts[0].IfName != "0/3" {
		t.Fatalf("FDB should attach to existing VLAN 10: %+v", byID[10])
	}
}

func TestBuildVLANInventory_fromPortsAndConfig(t *testing.T) {
	cfg := `
vlan database
vlan 1,10,190
vlan name 190 cameras
exit
interface 0/2
switchport mode access
switchport access vlan 190
exit
interface 0/1
switchport mode trunk
vlan tagging 10
exit
`
	ten := 10
	ports := []PortVLANHint{
		{IfIndex: 2, IfName: "0/2", Role: "access", AccessVLAN: intPtr(190)},
		{IfIndex: 5, IfName: "0/5", Role: "access", AccessVLAN: &ten, FDBVLAN: &ten},
	}
	inv := BuildVLANInventory(cfg, ports, []FDBVLANPort{{IfIndex: 5, VLANID: 10, IfName: "0/5"}})
	byID := map[int]VLANInventoryRow{}
	for _, r := range inv {
		byID[r.VLANID] = r
	}
	if !byID[190].InDatabase || byID[190].Name != "cameras" {
		t.Fatalf("190: %+v", byID[190])
	}
	if len(byID[190].AccessPorts) != 1 || byID[190].AccessPorts[0].IfName != "0/2" {
		t.Fatalf("190 access: %+v", byID[190].AccessPorts)
	}
	if len(byID[10].TaggedPorts) != 1 {
		t.Fatalf("10 tagged: %+v", byID[10].TaggedPorts)
	}
	tp := byID[10].TaggedPorts[0]
	if tp.IfName != "0/1" || tp.IfIndex != 0 {
		t.Fatalf("10 tagged unmapped port: %+v", tp)
	}
}

func TestBuildVLANInventory_unmappedTrunkTagged(t *testing.T) {
	cfg := `
vlan database
vlan 1,10
exit
interface 0/1
switchport mode trunk
vlan tagging 10
exit
`
	inv := BuildVLANInventory(cfg, nil, nil)
	byID := map[int]VLANInventoryRow{}
	for _, r := range inv {
		byID[r.VLANID] = r
	}
	if len(byID[10].TaggedPorts) != 1 || byID[10].TaggedPorts[0].IfName != "0/1" {
		t.Fatalf("expected unmapped tagged 0/1: %+v", byID[10].TaggedPorts)
	}
}

func TestVLANCLILines_ciscoThenIEEE(t *testing.T) {
	ch := PortVLANChange{Op: VLANOpSetAccess, VLANID: 10}
	cisco := strings.Join(VLANCLILines(vlanStyleCisco, ch), "\n")
	if !strings.Contains(cisco, "switchport mode access") || !strings.Contains(cisco, "switchport access vlan 10") {
		t.Fatalf("cisco: %s", cisco)
	}
	ieee := strings.Join(VLANCLILines(vlanStyleIEEE, ch), "\n")
	if !strings.Contains(ieee, "vlan pvid 10") || !strings.Contains(ieee, "vlan participation include 10") {
		t.Fatalf("ieee: %s", ieee)
	}
	add := VLANCLILines(vlanStyleCisco, PortVLANChange{Op: VLANOpAddTagged, VLANID: 20})
	if strings.Join(add, "\n") != "switchport mode trunk\nswitchport trunk allowed vlan add 20" {
		t.Fatalf("%v", add)
	}
	prev := 10
	rm := strings.Join(VLANCLILines(vlanStyleIEEE, PortVLANChange{Op: VLANOpRemove, VLANID: 10, PrevAccessVLAN: &prev}), "\n")
	if !strings.Contains(rm, "vlan participation exclude 10") || !strings.Contains(rm, "vlan pvid 1") {
		t.Fatalf("remove: %s", rm)
	}
}

func TestVLANCLILines_trunkAllow(t *testing.T) {
	add := VLANCLILines(vlanStyleCisco, PortVLANChange{
		Op: VLANOpTrunkAllow, AllowedMode: TrunkAllowAdd, AllowedList: "10;20-22",
	})
	wantAdd := "switchport mode trunk\nswitchport trunk allowed vlan add 10,20,21,22"
	if strings.Join(add, "\n") != wantAdd {
		t.Fatalf("add: %v", add)
	}
	all := strings.Join(VLANCLILines(vlanStyleCisco, PortVLANChange{
		Op: VLANOpTrunkAllow, AllowedMode: TrunkAllowAll,
	}), "\n")
	if all != "switchport mode trunk\nswitchport trunk allowed vlan all" {
		t.Fatalf("all: %s", all)
	}
	exc := strings.Join(VLANCLILines(vlanStyleCisco, PortVLANChange{
		Op: VLANOpTrunkAllow, AllowedMode: TrunkAllowExcept, AllowedList: "1,100",
	}), "\n")
	if !strings.Contains(exc, "switchport trunk allowed vlan except 1,100") {
		t.Fatalf("except: %s", exc)
	}
	ieee := VLANCLILines(vlanStyleIEEE, PortVLANChange{
		Op: VLANOpTrunkAllow, AllowedMode: TrunkAllowAdd, AllowedList: "10,20",
	})
	joined := strings.Join(ieee, "\n")
	if !strings.Contains(joined, "vlan tagging 10") || !strings.Contains(joined, "vlan tagging 20") {
		t.Fatalf("ieee add: %v", ieee)
	}
	if VLANCLILines(vlanStyleIEEE, PortVLANChange{Op: VLANOpTrunkAllow, AllowedMode: TrunkAllowAll}) != nil {
		t.Fatal("ieee has no allowed vlan all")
	}
	bad := PortVLANChange{Op: VLANOpTrunkAllow, AllowedMode: TrunkAllowAdd, AllowedList: ""}
	if err := bad.Validate(); err == nil {
		t.Fatal("empty allow list must fail")
	}
}

func TestNormalizeVLANList_semicolon(t *testing.T) {
	if got := FormatVLANIDList(ParseVLANIDList(NormalizeVLANList("10;20-22 30"))); got != "10,20,21,22,30" {
		t.Fatalf("got %q", got)
	}
}

func TestPortConfigBody_vlanCisco(t *testing.T) {
	ch := PortChange{VLAN: &PortVLANChange{Op: VLANOpSetAccess, VLANID: 15}, vlanStyle: vlanStyleCisco, Write: true}
	steps, err := portConfigBody(VendorUbiquiti, "0/3", ch)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(steps, "\n")
	if !strings.Contains(joined, "interface 0/3") || !strings.Contains(joined, "switchport access vlan 15") {
		t.Fatalf("%v", steps)
	}
	if strings.Contains(joined, "vlan pvid") {
		t.Fatal("cisco body should not include pvid")
	}
}

func TestVLANDatabaseCLILines(t *testing.T) {
	ch := VLANDatabaseChange{Op: VLANDBOpSetName, VLANID: 30, Name: "VLAN0030-DISP"}
	cisco := strings.Join(VLANDatabaseCLILines(vlanStyleCisco, ch), "\n")
	if !strings.Contains(cisco, "vlan 30") || !strings.Contains(cisco, "name ") {
		t.Fatalf("cisco name: %s", cisco)
	}
	if !strings.Contains(cisco, "'VLAN0030-DISP'") && !strings.Contains(cisco, `"VLAN0030-DISP"`) {
		t.Fatalf("cisco name must quote hyphens: %s", cisco)
	}
	ieee := strings.Join(VLANDatabaseCLILines(vlanStyleIEEE, ch), "\n")
	if !strings.Contains(ieee, "vlan database") || !strings.Contains(ieee, `vlan name 30 "VLAN0030-DISP"`) {
		t.Fatalf("ieee name: %s", ieee)
	}
	pechka := strings.Join(VLANDatabaseCLILines(vlanStyleIEEE, VLANDatabaseChange{
		Op: VLANDBOpSetName, VLANID: 16, Name: "VLAN-VIDEO-PECHKA-16",
	}), "\n")
	if !strings.Contains(pechka, `vlan name 16 "VLAN-VIDEO-PECHKA-16"`) {
		t.Fatalf("pechka: %s", pechka)
	}
	eltex := strings.Join(VLANDatabaseCLILines(vlanStyleEltex, VLANDatabaseChange{
		Op: VLANDBOpSetName, VLANID: 10, Name: "Management",
	}), "\n")
	if !strings.Contains(eltex, "vlan database") || !strings.Contains(eltex, "vlan 10 name Management") {
		t.Fatalf("eltex: %s", eltex)
	}
	hw := strings.Join(VLANDatabaseCLILines(vlanStyleHuawei, VLANDatabaseChange{
		Op: VLANDBOpSetName, VLANID: 10, Name: "IT-Department",
	}), "\n")
	if !strings.Contains(hw, "vlan 10") || !strings.Contains(hw, `name "IT-Department"`) || !strings.Contains(hw, "quit") {
		t.Fatalf("huawei: %s", hw)
	}
	hwDel := strings.Join(VLANDatabaseCLILines(vlanStyleHuawei, VLANDatabaseChange{Op: VLANDBOpDelete, VLANID: 10}), "\n")
	if hwDel != "undo vlan 10" {
		t.Fatalf("huawei del: %s", hwDel)
	}
	delIEEE := strings.Join(VLANDatabaseCLILines(vlanStyleIEEE, VLANDatabaseChange{Op: VLANDBOpDelete, VLANID: 30}), "\n")
	if !strings.Contains(delIEEE, "vlan database") || !strings.Contains(delIEEE, "no vlan 30") {
		t.Fatalf("delete ieee: %s", delIEEE)
	}
	delCisco := strings.Join(VLANDatabaseCLILines(vlanStyleCisco, VLANDatabaseChange{Op: VLANDBOpDelete, VLANID: 30}), "\n")
	if delCisco != "no vlan 30" {
		t.Fatalf("delete cisco: %s", delCisco)
	}
	clear := strings.Join(VLANDatabaseCLILines(vlanStyleCisco, VLANDatabaseChange{Op: VLANDBOpSetName, VLANID: 30, Name: ""}), "\n")
	if !strings.Contains(clear, "no name") {
		t.Fatalf("clear name: %s", clear)
	}
	bad := VLANDatabaseChange{Op: VLANDBOpDelete, VLANID: 1}
	if err := bad.Validate(); err == nil {
		t.Fatal("vlan 1 delete must fail")
	}
	bulkBad := VLANDatabaseChange{Op: VLANDBOpDelete, VLANIDs: []int{167, 1, 30}}
	if err := bulkBad.Validate(); err == nil {
		t.Fatal("bulk with vlan 1 must fail")
	}
	bulkIEEE := strings.Join(VLANDatabaseCLILines(vlanStyleIEEE, VLANDatabaseChange{
		Op: VLANDBOpDelete, VLANIDs: []int{167, 30, 31, 32},
	}), "\n")
	if bulkIEEE != "vlan database\nno vlan 30,31,32,167\nexit" {
		t.Fatalf("bulk ieee: %s", bulkIEEE)
	}
	bulkCisco := strings.Join(VLANDatabaseCLILines(vlanStyleCisco, VLANDatabaseChange{
		Op: VLANDBOpDelete, VLANIDs: []int{167, 30},
	}), "\n")
	if bulkCisco != "no vlan 30,167" {
		t.Fatalf("bulk cisco: %s", bulkCisco)
	}
	bulkHW := strings.Join(VLANDatabaseCLILines(vlanStyleHuawei, VLANDatabaseChange{
		Op: VLANDBOpDelete, VLANIDs: []int{20, 10},
	}), "\n")
	if bulkHW != "undo vlan 10\nundo vlan 20" {
		t.Fatalf("bulk huawei: %s", bulkHW)
	}
	createCisco := strings.Join(VLANDatabaseCLILines(vlanStyleCisco, VLANDatabaseChange{
		Op: VLANDBOpCreate, VLANID: 55, Name: "",
	}), "\n")
	if createCisco != "vlan 55\nexit" {
		t.Fatalf("create cisco bare: %s", createCisco)
	}
	createNamed := strings.Join(VLANDatabaseCLILines(vlanStyleEltex, VLANDatabaseChange{
		Op: VLANDBOpCreate, VLANID: 55, Name: "Office",
	}), "\n")
	if !strings.Contains(createNamed, "vlan database") || !strings.Contains(createNamed, "vlan 55 name Office") {
		t.Fatalf("create eltex: %s", createNamed)
	}
	createIEEE := strings.Join(VLANDatabaseCLILines(vlanStyleIEEE, VLANDatabaseChange{
		Op: VLANDBOpCreate, VLANID: 40, Name: "Cameras",
	}), "\n")
	if !strings.Contains(createIEEE, "vlan 40") || !strings.Contains(createIEEE, `vlan name 40 "Cameras"`) {
		t.Fatalf("create ieee: %s", createIEEE)
	}
}

func TestVLANDBAttemptsOrder(t *testing.T) {
	u := vlanDBAttempts(VendorUbiquiti)
	if len(u) < 1 || u[0].style != vlanStyleIEEE || u[0].enterConfig {
		t.Fatalf("ubiquiti first must be fastpath privileged: %+v", u)
	}
	e := vlanDBAttempts(VendorEltex)
	if len(e) < 1 || e[0].style != vlanStyleEltex {
		t.Fatalf("eltex first must be eltex-db: %+v", e)
	}
	h := vlanDBAttempts(VendorHuawei)
	if len(h) != 1 || h[0].style != vlanStyleHuawei {
		t.Fatalf("huawei: %+v", h)
	}
	c := vlanDBAttempts(VendorCisco)
	if len(c) < 1 || c[0].style != vlanStyleCisco {
		t.Fatalf("cisco first: %+v", c)
	}
}

func TestInterpretVLANDBCLI(t *testing.T) {
	if err := interpretVLANDBCLI("(Vlan)#\nvlan name 16 \"VLAN-VIDEO-PECHKA-16\"\n"); err != nil {
		t.Fatal(err)
	}
	if err := interpretVLANDBCLI("(config-vlan)#\nvlan name 30 Office\n"); err != nil {
		t.Fatal(err)
	}
	if err := interpretVLANDBCLI("(Vlan)#\n% Error: VLAN cannot be deleted — being used\n"); err == nil {
		t.Fatal("in-use vlan must fail")
	}
}

func intPtr(n int) *int { return &n }
