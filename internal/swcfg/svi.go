package swcfg

import (
	"net"
	"regexp"
	"strconv"
	"strings"
)

// SVIAddress — L3 адрес на VLAN-интерфейсе из show run.
type SVIAddress struct {
	VLANID    int    `json:"vlan_id"`
	IP        string `json:"ip"` // без маски
	IfaceName string `json:"iface_name,omitempty"`
}

var (
	reIfaceVLAN = regexp.MustCompile(`(?i)^(?:vlan(?:if)?|vlan-interface)\s*(\d+)$`)
	reIPAddr    = regexp.MustCompile(`(?i)^ip\s+address\s+(\d{1,3}(?:\.\d{1,3}){3})(?:/\d{1,2}|\s+\d{1,3}(?:\.\d{1,3}){3})?`)
	reIPAddrHuawei = regexp.MustCompile(`(?i)^ip\s+address\s+(\d{1,3}(?:\.\d{1,3}){3})\s+(\d{1,3}(?:\.\d{1,3}){3})`)
)

// VLANIDFromSVIIfaceName — Vlan100 / vlan 100 / Vlanif100 / Vlan-interface100 → id.
func VLANIDFromSVIIfaceName(name string) (int, bool) {
	n := strings.TrimSpace(name)
	n = strings.TrimPrefix(n, "interface ")
	n = strings.TrimSpace(n)
	// "Vlan 100" / "Vlan100"
	compact := strings.ReplaceAll(strings.ToLower(n), " ", "")
	compact = strings.ReplaceAll(compact, "-", "")
	for _, p := range []string{"vlanif", "vlaninterface", "vlan"} {
		if strings.HasPrefix(compact, p) {
			rest := compact[len(p):]
			id, err := strconv.Atoi(rest)
			if err == nil && id >= 1 && id <= 4094 {
				return id, true
			}
		}
	}
	m := reIfaceVLAN.FindStringSubmatch(strings.TrimSpace(n))
	if len(m) == 2 {
		id, err := strconv.Atoi(m[1])
		if err == nil && id >= 1 && id <= 4094 {
			return id, true
		}
	}
	return 0, false
}

// ParseSVIAddresses — interface Vlan* / Vlanif* + ip address из show running-config.
func ParseSVIAddresses(raw string) []SVIAddress {
	var out []SVIAddress
	var curVLAN int
	var curName string
	inSVI := false
	seen := map[string]struct{}{} // vlan:ip

	flushStop := func() {
		inSVI = false
		curVLAN = 0
		curName = ""
	}

	for _, rawLine := range strings.Split(raw, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		ll := strings.ToLower(line)
		if strings.HasPrefix(ll, "interface ") {
			flushStop()
			name := strings.TrimSpace(line[len("interface"):])
			if vid, ok := VLANIDFromSVIIfaceName(name); ok {
				inSVI = true
				curVLAN = vid
				curName = name
			}
			continue
		}
		if !inSVI {
			continue
		}
		if ll == "exit" || ll == "!" {
			flushStop()
			continue
		}
		ip, ok := parseIPv4FromAddressLine(ll)
		if !ok {
			continue
		}
		key := strconv.Itoa(curVLAN) + ":" + ip
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, SVIAddress{VLANID: curVLAN, IP: ip, IfaceName: curName})
	}
	return out
}

func parseIPv4FromAddressLine(ll string) (string, bool) {
	ll = strings.TrimSpace(ll)
	if strings.HasPrefix(ll, "no ip address") || strings.HasPrefix(ll, "undo ip address") {
		return "", false
	}
	if m := reIPAddr.FindStringSubmatch(ll); len(m) >= 2 {
		ip := m[1]
		if net.ParseIP(ip) == nil {
			return "", false
		}
		return ip, true
	}
	if m := reIPAddrHuawei.FindStringSubmatch(ll); len(m) >= 2 {
		ip := m[1]
		if net.ParseIP(ip) == nil {
			return "", false
		}
		return ip, true
	}
	return "", false
}

// NormalizeHostIP — host inventory → IPv4 строка для сравнения (без зоны/порта).
func NormalizeHostIP(host string) string {
	h := strings.TrimSpace(host)
	if h == "" {
		return ""
	}
	// [addr]:port или addr:port — только если выглядит как IPv4:port
	if i := strings.Index(h, "%"); i >= 0 {
		h = h[:i]
	}
	if ip := net.ParseIP(h); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			return v4.String()
		}
		return ""
	}
	// host:port где host — IPv4
	if hostOnly, _, err := net.SplitHostPort(h); err == nil {
		if ip := net.ParseIP(hostOnly); ip != nil {
			if v4 := ip.To4(); v4 != nil {
				return v4.String()
			}
		}
	}
	return ""
}

// MgmtVLANFromSVI — VLAN, у которого SVI IP совпадает с devices.host.
func MgmtVLANFromSVI(host string, svis []SVIAddress) (vlanID int, sviIP string, ok bool) {
	want := NormalizeHostIP(host)
	if want == "" {
		return 0, "", false
	}
	for _, s := range svis {
		if s.IP == want {
			return s.VLANID, s.IP, true
		}
	}
	return 0, "", false
}

// SVIForVLAN — есть ли SVI с IP на данном VLAN.
func SVIForVLAN(svis []SVIAddress, vlanID int) (SVIAddress, bool) {
	for _, s := range svis {
		if s.VLANID == vlanID && s.IP != "" {
			return s, true
		}
	}
	return SVIAddress{}, false
}
