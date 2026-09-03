package swcfg

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const (
	VLANOpSetAccess   = "set_access"
	VLANOpAddTagged   = "add_tagged" // устар.: один tagged; предпочтительно trunk_allow
	VLANOpRemove      = "remove"
	VLANOpTrunkAllow  = "trunk_allow"

	TrunkAllowAdd    = "add"
	TrunkAllowRemove = "remove"
	TrunkAllowAll    = "all"
	TrunkAllowExcept = "except"

	VLANDBOpSetName = "set_name"
	VLANDBOpDelete  = "delete"
	VLANDBOpCreate  = "create"
)

type vlanCLIStyle int

const (
	vlanStyleAuto vlanCLIStyle = iota
	vlanStyleCisco
	vlanStyleIEEE
	vlanStyleEltex // (config)# vlan database → vlan N name …
	vlanStyleHuawei
)

// PortVLANChange — прописать или снять VLAN на порту.
type PortVLANChange struct {
	Op             string
	VLANID         int    // set_access / remove / add_tagged
	AllowedMode    string // trunk_allow: add|remove|all|except
	AllowedList    string // trunk_allow: «10,20-22» (или ; как у SNR)
	PrevAccessVLAN *int
	PortMode       string // access | trunk | ""
}

func (v PortVLANChange) Validate() error {
	switch v.Op {
	case VLANOpSetAccess, VLANOpAddTagged, VLANOpRemove:
		if v.VLANID < 1 || v.VLANID > 4094 {
			return fmt.Errorf("vlan_id должен быть 1–4094")
		}
		return nil
	case VLANOpTrunkAllow:
		mode := strings.ToLower(strings.TrimSpace(v.AllowedMode))
		switch mode {
		case TrunkAllowAll:
			return nil
		case TrunkAllowAdd, TrunkAllowRemove, TrunkAllowExcept:
			ids := ParseVLANIDList(NormalizeVLANList(v.AllowedList))
			if len(ids) == 0 {
				return fmt.Errorf("укажите список VLAN в Allow vlan (например 10,20-22)")
			}
			if len(ids) > 256 {
				return fmt.Errorf("слишком много VLAN в списке (макс. 256)")
			}
			return nil
		default:
			return fmt.Errorf("allowed_mode: add | remove | all | except")
		}
	default:
		return fmt.Errorf("неизвестная операция VLAN %q", v.Op)
	}
}

// VLANDatabaseChange — имя, создание или удаление VLAN в vlan database свитча.
type VLANDatabaseChange struct {
	Op      string
	VLANID  int   // create / set_name / delete одного
	VLANIDs []int // delete нескольких: EdgeSwitch «no vlan 167,30,31»
	Name    string
}

func (v VLANDatabaseChange) Validate() error {
	switch v.Op {
	case VLANDBOpSetName, VLANDBOpCreate:
		if v.VLANID < 1 || v.VLANID > 4094 {
			return fmt.Errorf("vlan_id должен быть 1–4094")
		}
		return nil
	case VLANDBOpDelete:
		ids := v.DeleteIDs()
		if len(ids) == 0 {
			return fmt.Errorf("укажите vlan_id или vlan_ids")
		}
		if len(ids) > 256 {
			return fmt.Errorf("слишком много VLAN для удаления (макс. 256)")
		}
		for _, id := range ids {
			if id < 1 || id > 4094 {
				return fmt.Errorf("vlan_id должен быть 1–4094")
			}
			if id == 1 {
				return fmt.Errorf("VLAN 1 нельзя удалить")
			}
		}
		return nil
	default:
		return fmt.Errorf("неизвестная операция vlan database %q", v.Op)
	}
}

// DeleteIDs — уникальные отсортированные ID для удаления (VLANIDs или одиночный VLANID).
func (v VLANDatabaseChange) DeleteIDs() []int {
	if v.Op != VLANDBOpDelete {
		return nil
	}
	seen := map[int]struct{}{}
	var out []int
	add := func(id int) {
		if id < 1 {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	for _, id := range v.VLANIDs {
		add(id)
	}
	if len(out) == 0 {
		add(v.VLANID)
	}
	sort.Ints(out)
	return out
}

func (v VLANDatabaseChange) deleteIDCSV() string {
	return FormatVLANIDList(v.DeleteIDs())
}

func sanitizeVLANName(s string) string {
	return sanitizeVLANNameMax(s, 32)
}

func sanitizeVLANNameMax(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, `"`, "")
	s = strings.ReplaceAll(s, "'", "")
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	if maxRunes > 0 && len([]rune(s)) > maxRunes {
		r := []rune(s)
		s = string(r[:maxRunes])
	}
	return strings.TrimSpace(s)
}

// VLANPortRef — порт в составе VLAN.
type VLANPortRef struct {
	IfIndex int    `json:"if_index"`
	IfName  string `json:"if_name"`
	Role    string `json:"role,omitempty"`
}

// VLANInventoryRow — одна VLAN в карточке узла.
type VLANInventoryRow struct {
	VLANID      int           `json:"vlan_id"`
	Name        string        `json:"name,omitempty"`
	InDatabase  bool          `json:"in_database"`
	AccessPorts []VLANPortRef `json:"access_ports"`
	TaggedPorts []VLANPortRef `json:"tagged_ports"`
	FDBPorts    []VLANPortRef `json:"fdb_ports,omitempty"`
}

// PortVLANHint — то, что уже известно по порту без повторного SSH.
type PortVLANHint struct {
	IfIndex    int
	IfName     string
	Role       string
	AccessVLAN *int
	FDBVLAN    *int
}

// FDBVLANPort — пара порт/VLAN из FDB.
type FDBVLANPort struct {
	IfIndex int
	VLANID  int
	IfName  string
	Role    string
}

func validVLANID(n int) bool {
	return n >= 1 && n <= 4094
}

func addVLANID(dst map[int]struct{}, n int) {
	if validVLANID(n) {
		dst[n] = struct{}{}
	}
}

// NormalizeVLANList — «10;20-22 30» → «10,20-22,30» (SNR часто ;).
func NormalizeVLANList(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ";", ",")
	s = strings.ReplaceAll(s, " ", ",")
	for strings.Contains(s, ",,") {
		s = strings.ReplaceAll(s, ",,", ",")
	}
	return strings.Trim(s, ",")
}

// FormatVLANIDList — список ID в cisco/ELTEX виде через запятую.
func FormatVLANIDList(ids []int) string {
	if len(ids) == 0 {
		return ""
	}
	parts := make([]string, 0, len(ids))
	for _, n := range ids {
		parts = append(parts, strconv.Itoa(n))
	}
	return strings.Join(parts, ",")
}

// ParseVLANIDList разбирает «10,20-22,30» (и «10;20» после Normalize).
func ParseVLANIDList(s string) []int {
	s = NormalizeVLANList(s)
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.TrimPrefix(s, "add ")
	s = strings.TrimPrefix(s, "remove ")
	s = strings.TrimPrefix(s, "except ")
	s = strings.TrimSpace(s)
	if s == "" || s == "all" || s == "none" {
		return nil
	}
	seen := map[int]struct{}{}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if i := strings.IndexByte(part, '-'); i > 0 {
			a, err1 := strconv.Atoi(strings.TrimSpace(part[:i]))
			b, err2 := strconv.Atoi(strings.TrimSpace(part[i+1:]))
			if err1 != nil || err2 != nil || a < 1 || b < 1 {
				continue
			}
			if a > b {
				a, b = b, a
			}
			if b-a > 4094 {
				continue
			}
			for n := a; n <= b; n++ {
				addVLANID(seen, n)
			}
			continue
		}
		n, err := strconv.Atoi(part)
		if err == nil {
			addVLANID(seen, n)
		}
	}
	out := make([]int, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Ints(out)
	return out
}

func unquoteVLANName(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '\'' && s[len(s)-1] == '\'') || (s[0] == '"' && s[len(s)-1] == '"') {
			return strings.TrimSpace(s[1 : len(s)-1])
		}
	}
	return s
}

// ParseVLANDatabase — ID и имена из vlan database / IOS vlan N / name.
func ParseVLANDatabase(raw string) map[int]string {
	names := map[int]string{}
	inDB := false
	inIface := false
	iosVLAN := 0
	for _, rawLine := range strings.Split(raw, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		ll := strings.ToLower(line)
		if strings.HasPrefix(ll, "interface ") {
			inIface = true
			inDB = false
			iosVLAN = 0
			continue
		}
		if inIface && (ll == "exit" || ll == "!" || ll == "end") {
			inIface = false
			continue
		}
		if inIface {
			continue
		}
		if ll == "vlan database" {
			inDB = true
			iosVLAN = 0
			continue
		}
		if inDB && (ll == "exit" || ll == "end") {
			inDB = false
			continue
		}
		if inDB {
			if strings.HasPrefix(ll, "vlan name ") {
				rest := strings.TrimSpace(line[len("vlan name"):])
				idStr, name, _ := strings.Cut(rest, " ")
				if n, err := strconv.Atoi(strings.TrimSpace(idStr)); err == nil && validVLANID(n) {
					names[n] = unquoteVLANName(name)
				}
				continue
			}
			if strings.HasPrefix(ll, "vlan ") && !strings.HasPrefix(ll, "vlan name") {
				rest := strings.TrimSpace(line[len("vlan"):])
				if n, name, ok := parseVLANIDNameRest(rest); ok {
					names[n] = name
					continue
				}
				for _, n := range ParseVLANIDList(rest) {
					if _, ok := names[n]; !ok {
						names[n] = ""
					}
				}
			}
			continue
		}
		if ll == "!" {
			iosVLAN = 0
			continue
		}
		if strings.HasPrefix(ll, "vlan ") && !strings.HasPrefix(ll, "vlan name") {
			rest := strings.TrimSpace(line[len("vlan"):])
			if n, name, ok := parseVLANIDNameRest(rest); ok {
				names[n] = name
				iosVLAN = n
				continue
			}
			if !strings.Contains(ll, ",") && !strings.Contains(ll, "-") {
				if n, err := strconv.Atoi(strings.TrimSpace(rest)); err == nil && validVLANID(n) {
					iosVLAN = n
					if _, ok := names[n]; !ok {
						names[n] = ""
					}
				}
			}
			continue
		}
		if iosVLAN > 0 && strings.HasPrefix(ll, "name ") {
			names[iosVLAN] = unquoteVLANName(strings.TrimSpace(line[len("name"):]))
		}
	}
	return names
}

// parseVLANIDNameRest — «10 name Office» / «10 name "Cameras"» (ELTEX MES / HP one-liner).
func parseVLANIDNameRest(rest string) (id int, name string, ok bool) {
	rest = strings.TrimSpace(rest)
	low := strings.ToLower(rest)
	idx := strings.Index(low, " name ")
	if idx < 0 {
		return 0, "", false
	}
	idStr := strings.TrimSpace(rest[:idx])
	if strings.ContainsAny(idStr, ",-") {
		return 0, "", false
	}
	n, err := strconv.Atoi(idStr)
	if err != nil || !validVLANID(n) {
		return 0, "", false
	}
	return n, unquoteVLANName(strings.TrimSpace(rest[idx+len(" name "):])), true
}

func applyVLANIfaceLine(cur *ParsedInterfaceCLI, ll string) {
	if strings.HasPrefix(ll, "vlan pvid ") {
		rest := strings.TrimSpace(ll[len("vlan pvid"):])
		if n, err := strconv.Atoi(rest); err == nil && validVLANID(n) {
			cur.PVID = &n
			if cur.AccessVLAN == nil {
				cur.AccessVLAN = &n
			}
		}
		return
	}
	if strings.HasPrefix(ll, "vlan participation include ") {
		cur.Include = appendUniqueInts(cur.Include, ParseVLANIDList(strings.TrimSpace(ll[len("vlan participation include"):]))...)
		return
	}
	if strings.HasPrefix(ll, "vlan tagging ") {
		cur.Tagged = appendUniqueInts(cur.Tagged, ParseVLANIDList(strings.TrimSpace(ll[len("vlan tagging"):]))...)
		return
	}
	if strings.HasPrefix(ll, "switchport trunk native vlan ") {
		rest := strings.TrimSpace(ll[len("switchport trunk native vlan"):])
		if n, err := strconv.Atoi(rest); err == nil && validVLANID(n) {
			cur.TrunkNative = &n
			if cur.PVID == nil {
				cur.PVID = &n
			}
		}
		return
	}
	if strings.HasPrefix(ll, "switchport trunk allowed vlan ") {
		rest := strings.TrimSpace(ll[len("switchport trunk allowed vlan"):])
		if rest == "all" {
			cur.TrunkAllowedAll = true
			return
		}
		cur.TrunkAllowed = appendUniqueInts(cur.TrunkAllowed, ParseVLANIDList(rest)...)
	}
}

func appendUniqueInts(dst []int, add ...int) []int {
	seen := map[int]struct{}{}
	for _, n := range dst {
		seen[n] = struct{}{}
	}
	for _, n := range add {
		if !validVLANID(n) {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		dst = append(dst, n)
	}
	sort.Ints(dst)
	return dst
}

func portName(h PortVLANHint) string {
	if strings.TrimSpace(h.IfName) != "" {
		return h.IfName
	}
	return fmt.Sprintf("ifIndex %d", h.IfIndex)
}

func refFromHint(h PortVLANHint) VLANPortRef {
	return VLANPortRef{IfIndex: h.IfIndex, IfName: portName(h), Role: strings.ToLower(strings.TrimSpace(h.Role))}
}

func ensureRow(rows map[int]*VLANInventoryRow, id int) *VLANInventoryRow {
	if r, ok := rows[id]; ok {
		return r
	}
	r := &VLANInventoryRow{VLANID: id, AccessPorts: []VLANPortRef{}, TaggedPorts: []VLANPortRef{}}
	rows[id] = r
	return r
}

func hasPort(list []VLANPortRef, ifIndex int) bool {
	for _, p := range list {
		if p.IfIndex == ifIndex {
			return true
		}
	}
	return false
}

// hasPortRef — дубликат по if_index (>0) или по имени (для портов только из show run).
func hasPortRef(list []VLANPortRef, ref VLANPortRef) bool {
	nameKey := NormalizeIfaceKey(ref.IfName)
	for _, p := range list {
		if ref.IfIndex > 0 && p.IfIndex == ref.IfIndex {
			return true
		}
		if nameKey != "" && NormalizeIfaceKey(p.IfName) == nameKey {
			return true
		}
	}
	return false
}

// BuildVLANInventory собирает VLAN database + членство портов из show run и уже известных полей портов/FDB.
func BuildVLANInventory(config string, ports []PortVLANHint, fdb []FDBVLANPort) []VLANInventoryRow {
	rows := map[int]*VLANInventoryRow{}
	for id, name := range ParseVLANDatabase(config) {
		r := ensureRow(rows, id)
		r.InDatabase = true
		r.Name = name
	}
	parsed := ParseRunningConfigPortModes(config)
	portByIndex := map[int]PortVLANHint{}
	ifNames := map[int]string{}
	for _, p := range ports {
		portByIndex[p.IfIndex] = p
		if p.IfName != "" {
			ifNames[p.IfIndex] = p.IfName
		}
	}
	for _, block := range parsed {
		idx, ok := MatchConfigIfaceToIfIndex(block.IfaceName, ifNames)
		var h PortVLANHint
		if ok {
			h = portByIndex[idx]
			h.IfIndex = idx
			if h.IfName == "" {
				h.IfName = block.IfaceName
			}
		} else {
			// Порт есть в show run, но ещё нет в device_interfaces — не теряем membership.
			name := strings.TrimSpace(block.IfaceName)
			if name == "" {
				continue
			}
			h = PortVLANHint{IfIndex: 0, IfName: name}
		}
		role := PortRoleFromCLIMode(block.Mode)
		if role != "" {
			h.Role = role
		}
		access := block.AccessVLAN
		if access == nil {
			access = block.PVID
		}
		if access == nil {
			access = block.TrunkNative
		}
		if access != nil && (role == "access" || role == "") {
			r := ensureRow(rows, *access)
			rf := refFromHint(h)
			rf.Role = "access"
			if !hasPortRef(r.AccessPorts, rf) {
				r.AccessPorts = append(r.AccessPorts, rf)
			}
		}
		tagged := append([]int{}, block.Tagged...)
		if role == "trunk" {
			tagged = appendUniqueInts(tagged, block.TrunkAllowed...)
			tagged = appendUniqueInts(tagged, block.Include...)
			if access != nil {
				filtered := tagged[:0]
				for _, n := range tagged {
					if n != *access {
						filtered = append(filtered, n)
					}
				}
				tagged = filtered
			}
		} else {
			tagged = appendUniqueInts(tagged, block.Tagged...)
		}
		for _, vid := range tagged {
			if access != nil && vid == *access && role != "trunk" {
				continue
			}
			r := ensureRow(rows, vid)
			rf := refFromHint(h)
			rf.Role = "trunk"
			if hasPortRef(r.AccessPorts, rf) || hasPortRef(r.TaggedPorts, rf) {
				continue
			}
			r.TaggedPorts = append(r.TaggedPorts, rf)
		}
	}
	// Access VLAN из БД (cli_access_vlan) — только для портов, которых не было в show run.
	// Иначе после ручного удаления VLAN / перечитывания конфига устаревший кэш рисует «· с портов».
	seenInConfig := map[int]struct{}{}
	for _, block := range parsed {
		if idx, ok := MatchConfigIfaceToIfIndex(block.IfaceName, ifNames); ok {
			seenInConfig[idx] = struct{}{}
		}
	}
	for _, p := range ports {
		if _, ok := seenInConfig[p.IfIndex]; ok {
			continue
		}
		if p.AccessVLAN != nil && validVLANID(*p.AccessVLAN) {
			r := ensureRow(rows, *p.AccessVLAN)
			if !hasPort(r.AccessPorts, p.IfIndex) {
				rf := refFromHint(p)
				if rf.Role == "" {
					rf.Role = "access"
				}
				r.AccessPorts = append(r.AccessPorts, rf)
			}
		}
	}
	for _, e := range fdb {
		if !validVLANID(e.VLANID) || e.IfIndex <= 0 {
			continue
		}
		// Не создавать «призраков» только из FDB: после no vlan N MAC могут жить ещё долго.
		r, ok := rows[e.VLANID]
		if !ok {
			continue
		}
		if hasPort(r.AccessPorts, e.IfIndex) || hasPort(r.TaggedPorts, e.IfIndex) || hasPort(r.FDBPorts, e.IfIndex) {
			continue
		}
		name := e.IfName
		if name == "" {
			if h, ok := portByIndex[e.IfIndex]; ok {
				name = portName(h)
			} else {
				name = fmt.Sprintf("ifIndex %d", e.IfIndex)
			}
		}
		rf := VLANPortRef{IfIndex: e.IfIndex, IfName: name, Role: e.Role}
		if hasPortRef(r.AccessPorts, rf) || hasPortRef(r.TaggedPorts, rf) || hasPort(r.FDBPorts, e.IfIndex) {
			continue
		}
		r.FDBPorts = append(r.FDBPorts, rf)
	}
	out := make([]VLANInventoryRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].VLANID < out[j].VLANID })
	return out
}

// VLANConfiguredOnPorts — VLAN прописан на порту в show run / access-hint (не FDB).
func VLANConfiguredOnPorts(r VLANInventoryRow) bool {
	return len(r.AccessPorts) > 0 || len(r.TaggedPorts) > 0
}

// FormatVLANPortBindings — «0/2 (access), 0/5 (trunk)» для ошибок удаления.
func FormatVLANPortBindings(r VLANInventoryRow) string {
	var parts []string
	for _, p := range r.AccessPorts {
		n := p.IfName
		if n == "" {
			n = fmt.Sprintf("ifIndex %d", p.IfIndex)
		}
		parts = append(parts, n+" (access)")
	}
	for _, p := range r.TaggedPorts {
		n := p.IfName
		if n == "" {
			n = fmt.Sprintf("ifIndex %d", p.IfIndex)
		}
		parts = append(parts, n+" (tagged)")
	}
	return strings.Join(parts, ", ")
}

// VLANCLILines — команды на порту: сначала cisco-style, иначе IEEE 802.1Q (PVID / tagged / untagged).
func VLANCLILines(style vlanCLIStyle, ch PortVLANChange) []string {
	if style == vlanStyleIEEE {
		return vlanIEEELines(ch)
	}
	return vlanCiscoLines(ch)
}

func vlanCiscoLines(ch PortVLANChange) []string {
	id := strconv.Itoa(ch.VLANID)
	switch ch.Op {
	case VLANOpSetAccess:
		return []string{
			"switchport mode access",
			"switchport access vlan " + id,
		}
	case VLANOpAddTagged:
		return []string{
			"switchport mode trunk",
			"switchport trunk allowed vlan add " + id,
		}
	case VLANOpTrunkAllow:
		mode := strings.ToLower(strings.TrimSpace(ch.AllowedMode))
		lines := []string{"switchport mode trunk"}
		switch mode {
		case TrunkAllowAll:
			// EdgeSwitch / ELTEX MES23xx / SNR: all
			lines = append(lines, "switchport trunk allowed vlan all")
		case TrunkAllowAdd:
			list := FormatVLANIDList(ParseVLANIDList(ch.AllowedList))
			lines = append(lines, "switchport trunk allowed vlan add "+list)
		case TrunkAllowRemove:
			list := FormatVLANIDList(ParseVLANIDList(ch.AllowedList))
			lines = append(lines, "switchport trunk allowed vlan remove "+list)
		case TrunkAllowExcept:
			list := FormatVLANIDList(ParseVLANIDList(ch.AllowedList))
			// EdgeSwitch / SNR; ELTEX — add/remove/all, except может дать Invalid input → IEEE/другой fallback
			lines = append(lines, "switchport trunk allowed vlan except "+list)
		}
		return lines
	case VLANOpRemove:
		mode := strings.ToLower(strings.TrimSpace(ch.PortMode))
		if mode == "access" || (ch.PrevAccessVLAN != nil && *ch.PrevAccessVLAN == ch.VLANID) {
			return []string{"switchport access vlan 1"}
		}
		return []string{"switchport trunk allowed vlan remove " + id}
	default:
		return nil
	}
}

func vlanIEEELines(ch PortVLANChange) []string {
	id := strconv.Itoa(ch.VLANID)
	switch ch.Op {
	case VLANOpSetAccess:
		lines := []string{
			"vlan pvid " + id,
			"vlan participation include " + id,
			"no vlan tagging " + id,
		}
		if ch.PrevAccessVLAN != nil && *ch.PrevAccessVLAN != ch.VLANID && validVLANID(*ch.PrevAccessVLAN) {
			lines = append(lines, "vlan participation exclude "+strconv.Itoa(*ch.PrevAccessVLAN))
		}
		return lines
	case VLANOpAddTagged:
		return []string{
			"vlan participation include " + id,
			"vlan tagging " + id,
		}
	case VLANOpTrunkAllow:
		mode := strings.ToLower(strings.TrimSpace(ch.AllowedMode))
		// Fastpath: нет allowed vlan all/except — только tagging/participation по ID.
		if mode == TrunkAllowAll || mode == TrunkAllowExcept {
			return nil
		}
		ids := ParseVLANIDList(ch.AllowedList)
		var lines []string
		for _, n := range ids {
			sid := strconv.Itoa(n)
			if mode == TrunkAllowRemove {
				lines = append(lines, "vlan participation exclude "+sid, "no vlan tagging "+sid)
			} else {
				lines = append(lines, "vlan participation include "+sid, "vlan tagging "+sid)
			}
		}
		return lines
	case VLANOpRemove:
		lines := []string{
			"vlan participation exclude " + id,
			"no vlan tagging " + id,
		}
		if ch.PrevAccessVLAN != nil && *ch.PrevAccessVLAN == ch.VLANID {
			lines = append(lines, "vlan pvid 1")
		}
		return lines
	default:
		return nil
	}
}

// VLANDatabaseCLILines — команды по стилю CLI вендора.
func VLANDatabaseCLILines(style vlanCLIStyle, ch VLANDatabaseChange) []string {
	switch style {
	case vlanStyleIEEE:
		return vlanDBIEEELines(ch)
	case vlanStyleEltex:
		return vlanDBEltexLines(ch)
	case vlanStyleHuawei:
		return vlanDBHuaweiLines(ch)
	default:
		return vlanDBCiscoLines(ch)
	}
}

func vlanDBCiscoLines(ch VLANDatabaseChange) []string {
	id := strconv.Itoa(ch.VLANID)
	switch ch.Op {
	case VLANDBOpCreate:
		name := sanitizeVLANName(ch.Name)
		if name == "" {
			return []string{"vlan " + id, "exit"}
		}
		return []string{"vlan " + id, "name " + quoteCLIDescription(name), "exit"}
	case VLANDBOpSetName:
		name := sanitizeVLANName(ch.Name)
		if name == "" {
			return []string{"vlan " + id, "no name", "exit"}
		}
		return []string{"vlan " + id, "name " + quoteCLIDescription(name), "exit"}
	case VLANDBOpDelete:
		// IOS / SNR: «no vlan 10,20» или по одному — список через запятую.
		return []string{"no vlan " + ch.deleteIDCSV()}
	default:
		return nil
	}
}

func vlanDBIEEELines(ch VLANDatabaseChange) []string {
	id := strconv.Itoa(ch.VLANID)
	switch ch.Op {
	case VLANDBOpCreate:
		name := sanitizeVLANName(ch.Name)
		if name == "" {
			return []string{"vlan database", "vlan " + id, "exit"}
		}
		return []string{"vlan database", "vlan " + id, "vlan name " + id + " " + quoteVLANDBName(name), "exit"}
	case VLANDBOpSetName:
		name := sanitizeVLANName(ch.Name)
		if name == "" {
			return []string{"vlan database", "no vlan name " + id, "exit"}
		}
		// EdgeSwitch: vlan name из (Vlan)#; имя с дефисами/пробелами — в двойных кавычках.
		return []string{"vlan database", "vlan " + id, "vlan name " + id + " " + quoteVLANDBName(name), "exit"}
	case VLANDBOpDelete:
		// EdgeSwitch: (Vlan)# no vlan 167,30,31,32
		return []string{"vlan database", "no vlan " + ch.deleteIDCSV(), "exit"}
	default:
		return nil
	}
}

// vlanDBEltexLines — MES23xx/33xx: (config)# vlan database → vlan N name ….
func vlanDBEltexLines(ch VLANDatabaseChange) []string {
	id := strconv.Itoa(ch.VLANID)
	switch ch.Op {
	case VLANDBOpCreate:
		name := sanitizeVLANName(ch.Name)
		if name == "" {
			return []string{"vlan database", "vlan " + id, "exit"}
		}
		return []string{"vlan database", "vlan " + id + " name " + quoteCLIDescription(name), "exit"}
	case VLANDBOpSetName:
		name := sanitizeVLANName(ch.Name)
		if name == "" {
			return []string{"vlan database", "vlan " + id, "no name", "exit"}
		}
		return []string{"vlan database", "vlan " + id + " name " + quoteCLIDescription(name), "exit"}
	case VLANDBOpDelete:
		return []string{"vlan database", "no vlan " + ch.deleteIDCSV(), "exit"}
	default:
		return nil
	}
}

// vlanDBHuaweiLines — VRP: system-view → vlan N → name / undo name → quit; undo vlan N.
func vlanDBHuaweiLines(ch VLANDatabaseChange) []string {
	id := strconv.Itoa(ch.VLANID)
	switch ch.Op {
	case VLANDBOpCreate:
		name := sanitizeVLANNameMax(ch.Name, 31)
		if name == "" {
			return []string{"vlan " + id, "quit"}
		}
		return []string{"vlan " + id, "name " + quoteVLANDBName(name), "quit"}
	case VLANDBOpSetName:
		name := sanitizeVLANNameMax(ch.Name, 31)
		if name == "" {
			return []string{"vlan " + id, "undo name", "quit"}
		}
		return []string{"vlan " + id, "name " + quoteVLANDBName(name), "quit"}
	case VLANDBOpDelete:
		// VRP: по одному undo vlan N (списка в одной команде нет).
		var lines []string
		for _, n := range ch.DeleteIDs() {
			lines = append(lines, "undo vlan "+strconv.Itoa(n))
		}
		return lines
	default:
		return nil
	}
}

// quoteVLANDBName — Fastpath/EdgeSwitch/Huawei: всегда "…", иначе дефисы дают Invalid input.
func quoteVLANDBName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return `""`
	}
	return `"` + strings.ReplaceAll(name, `"`, "") + `"`
}

func interpretVLANDBCLI(out string) error {
	low := strings.ToLower(out)
	// EdgeSwitch: (Vlan)# без (config. Ищем срез после входа в режим конфига/vlan.
	idx := strings.Index(low, "(config")
	if idx < 0 {
		idx = strings.Index(low, "config)#")
	}
	if idx < 0 {
		idx = strings.Index(low, "(vlan")
	}
	if idx < 0 {
		idx = strings.Index(low, "vlan)#")
	}
	check := low
	if idx >= 0 {
		check = low[idx:]
	}
	for _, bad := range []string{
		"invalid input detected",
		"incomplete command",
		"command not found",
		"unknown command",
		"% invalid",
		"permission denied",
		"authorization failed",
		"cannot be deleted",
		"cannot delete",
		"being used",
	} {
		if strings.Contains(check, bad) {
			return fmt.Errorf("cli: %s — %s", bad, compactCLIErr(out))
		}
	}
	if err := interpretPortCLI(out); err == nil {
		return nil
	}
	ok := strings.Contains(low, "vlan database") ||
		strings.Contains(low, "vlan name") ||
		strings.Contains(low, "no vlan") ||
		strings.Contains(low, "undo vlan") ||
		strings.Contains(low, "undo name") ||
		strings.Contains(low, "(vlan") ||
		strings.Contains(low, "vlan)#") ||
		strings.Contains(low, "-vlan") ||
		strings.Contains(low, "[*")
	if ok {
		return nil
	}
	return interpretPortCLI(out)
}

func cliLooksUnsupported(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	for _, p := range []string{
		"invalid input", "unknown command", "incomplete command",
		"command not found", "% invalid", "invalid command",
		"unrecognized command", "wrong parameter",
	} {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}
