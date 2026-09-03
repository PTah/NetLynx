package swcfg

import (
	"fmt"
	"strconv"
	"strings"
)

// PortCLIMode — switchport mode из running-config (interface block).
type PortCLIMode struct {
	Mode       string // access | trunk | …
	AccessVLAN *int
}

// ParsedInterfaceCLI — один блок interface из show run.
type ParsedInterfaceCLI struct {
	IfaceName   string
	Description string // из «description …»; пусто если строки не было
	HasDescr    bool   // true, если встретили description / no description
	PortCLIMode
	PVID             *int
	Include          []int
	Tagged           []int
	TrunkAllowed     []int
	TrunkAllowedAll  bool
	TrunkNative      *int
}

// ParseRunningConfigPortModes разбирает полный show running-config.
// Ключ map — нормализованное имя интерфейса (см. NormalizeIfaceKey).
func ParseRunningConfigPortModes(raw string) map[string]ParsedInterfaceCLI {
	out := make(map[string]ParsedInterfaceCLI)
	var cur *ParsedInterfaceCLI
	flush := func() {
		if cur == nil || strings.TrimSpace(cur.IfaceName) == "" {
			return
		}
		key := NormalizeIfaceKey(cur.IfaceName)
		if key == "" {
			return
		}
		out[key] = *cur
	}
	for _, rawLine := range strings.Split(raw, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		ll := strings.ToLower(line)
		if strings.HasPrefix(ll, "interface ") {
			flush()
			name := strings.TrimSpace(line[len("interface"):])
			cur = &ParsedInterfaceCLI{IfaceName: name}
			continue
		}
		if cur == nil {
			continue
		}
		if ll == "exit" || ll == "!" {
			flush()
			cur = nil
			continue
		}
		if applyDescriptionLine(cur, line, ll) {
			continue
		}
		applySwitchportLine(cur, ll)
		applyVLANIfaceLine(cur, ll)
	}
	flush()
	return out
}

func applyDescriptionLine(cur *ParsedInterfaceCLI, line, ll string) bool {
	if ll == "no description" || ll == "undo description" {
		cur.HasDescr = true
		cur.Description = ""
		return true
	}
	if !strings.HasPrefix(ll, "description ") && ll != "description" {
		return false
	}
	cur.HasDescr = true
	rest := strings.TrimSpace(line)
	// сохраняем регистр текста: отрезаем префикс по длине ключевого слова
	if len(rest) >= len("description") {
		rest = strings.TrimSpace(rest[len("description"):])
	} else {
		rest = ""
	}
	cur.Description = unquoteCLIDescription(rest)
	return true
}

// unquoteCLIDescription снимает кавычки EdgeSwitch/Cisco: 'foo' / "foo".
func unquoteCLIDescription(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '\'' && s[len(s)-1] == '\'') || (s[0] == '"' && s[len(s)-1] == '"') {
			return strings.TrimSpace(s[1 : len(s)-1])
		}
	}
	return s
}

// LooksLikeHardwareIfDescr — SNMP ifDescr без ifAlias (EdgeSwitch «Slot: 0 Port: N Gigabit - Level»).
func LooksLikeHardwareIfDescr(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return true
	}
	low := strings.ToLower(s)
	if strings.HasPrefix(low, "slot:") && strings.Contains(low, "port:") {
		return true
	}
	if strings.Contains(low, "gigabit - level") || strings.Contains(low, "cpu interface") {
		return true
	}
	return false
}

func applySwitchportLine(cur *ParsedInterfaceCLI, ll string) {
	if strings.HasPrefix(ll, "switchport mode ") {
		mode := strings.TrimSpace(ll[len("switchport mode"):])
		if mode != "" {
			cur.Mode = mode
		}
		return
	}
	if strings.HasPrefix(ll, "switchport access vlan ") {
		rest := strings.TrimSpace(ll[len("switchport access vlan"):])
		if n, err := strconv.Atoi(rest); err == nil && n > 0 && n <= 4094 {
			cur.AccessVLAN = &n
		}
		return
	}
	// native VLAN on trunk (informational; role stays trunk)
	if strings.HasPrefix(ll, "switchport trunk native vlan ") {
		return
	}
}

// NormalizeIfaceKey — сопоставление имён из CLI и SNMP ifName.
func NormalizeIfaceKey(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.TrimPrefix(s, "interface ")
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	for _, p := range []string{
		"gigabitethernet", "tengigabitethernet", "fortygigabitethernet",
		"fastethernet", "ethernet", "port-channel", "po",
	} {
		if strings.HasPrefix(s, p) {
			s = strings.TrimPrefix(s, p)
			break
		}
	}
	for _, p := range []string{"gi", "te", "fa", "fo", "xe", "hu", "eth"} {
		if strings.HasPrefix(s, p) && len(s) > len(p) {
			s = s[len(p):]
			break
		}
	}
	return strings.TrimSpace(s)
}

// MatchConfigIfaceToIfIndex сопоставляет имя из конфига с if_index по SNMP if_name.
func MatchConfigIfaceToIfIndex(configName string, ifNames map[int]string) (int, bool) {
	key := NormalizeIfaceKey(configName)
	if key == "" {
		return 0, false
	}
	for idx, name := range ifNames {
		if NormalizeIfaceKey(name) == key {
			return idx, true
		}
	}
	// fallback: совпадение хвоста slot/port (0/17)
	for idx, name := range ifNames {
		if ifaceTailMatch(key, NormalizeIfaceKey(name)) {
			return idx, true
		}
	}
	return 0, false
}

func ifaceTailMatch(a, b string) bool {
	if a == b {
		return true
	}
	partsA := strings.Split(a, "/")
	partsB := strings.Split(b, "/")
	if len(partsA) >= 2 && len(partsB) >= 2 {
		ta := partsA[len(partsA)-2] + "/" + partsA[len(partsA)-1]
		tb := partsB[len(partsB)-2] + "/" + partsB[len(partsB)-1]
		return ta == tb
	}
	return false
}

// PortRoleFromCLIMode — operational port_role для NetLynx (access/trunk).
func PortRoleFromCLIMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "trunk", "dot1q-tunnel", "private-vlan":
		return "trunk"
	case "access", "hybrid":
		return "access"
	default:
		if mode != "" {
			return "access"
		}
		return ""
	}
}

// ParseInterfacePortCLIMode — mode из snippet одного interface (reuse для live settings).
func ParseInterfacePortCLIMode(out string) PortCLIMode {
	var cur ParsedInterfaceCLI
	low := strings.ToLower(out)
	idx := strings.Index(low, "interface ")
	body := out
	if idx >= 0 {
		body = out[idx:]
	}
	for _, raw := range strings.Split(body, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		ll := strings.ToLower(line)
		if strings.HasPrefix(ll, "interface ") {
			cur.IfaceName = strings.TrimSpace(line[len("interface"):])
			continue
		}
		if ll == "exit" {
			break
		}
		applySwitchportLine(&cur, ll)
		applyVLANIfaceLine(&cur, ll)
	}
	return cur.PortCLIMode
}

func (m PortCLIMode) String() string {
	if m.AccessVLAN != nil {
		return fmt.Sprintf("%s vlan=%d", m.Mode, *m.AccessVLAN)
	}
	return m.Mode
}
