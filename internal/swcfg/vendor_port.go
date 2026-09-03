package swcfg

import (
	"fmt"
	"strings"
)

// SupportsPortCLI — запись настроек порта через SSH (не XP).
func SupportsPortCLI(v Vendor, sysDescr, name string) bool {
	if isEdgeSwitchXP(sysDescr, name) {
		return false
	}
	switch v {
	case VendorUbiquiti, VendorEltex, VendorSNR, VendorMikrotik,
		VendorCisco, VendorAruba, VendorZyxel, VendorHuawei,
		VendorHP, VendorTPLink, VendorDLink,
		VendorDahua, VendorHikvision, VendorHiWatch, VendorTrassir:
		return true
	case VendorAuto:
		d := DetectVendor("", sysDescr, name)
		return SupportsPortCLI(d, sysDescr, name)
	default:
		return false
	}
}

// SupportsPoE24V — только EdgeSwitch Fastpath.
func SupportsPoE24V(v Vendor, sysDescr, name string) bool {
	if isEdgeSwitchXP(sysDescr, name) {
		return false
	}
	d := v
	if d == VendorAuto {
		d = DetectVendor("", sysDescr, name)
	}
	return d == VendorUbiquiti
}

func confEnterCmds(v Vendor) (priv, conf, write string) {
	switch v {
	case VendorEltex:
		return "enable", "configure terminal", "write"
	case VendorSNR:
		return "enable", "configure terminal", "write memory"
	case VendorCisco, VendorAruba, VendorZyxel, VendorHP, VendorTPLink, VendorDLink,
		VendorDahua, VendorHikvision, VendorHiWatch, VendorTrassir:
		return "enable", "configure terminal", "write memory"
	case VendorHuawei:
		// VRP: без классического enable; правки в system-view, save.
		return "", "system-view", "save"
	default:
		return "en", "configure", "write memory"
	}
}

// ifaceExitCmd / confExitCmd — выход из interface / config режима.
func ifaceExitCmd(v Vendor) string {
	if v == VendorHuawei {
		return "quit"
	}
	return "exit"
}

func confExitCmd(v Vendor) string {
	if v == VendorHuawei {
		return "return"
	}
	return "exit"
}

// PoEModeCLI — команда(ы) PoE для вендора.
func PoEModeCLI(v Vendor, mode string) (string, error) {
	m, err := NormalizePoEMode(mode)
	if err != nil {
		return "", err
	}
	switch v {
	case VendorEltex:
		switch m {
		case "off":
			return "power inline never", nil
		case "poe+":
			return "power inline auto", nil
		case "24v":
			return "", fmt.Errorf("Eltex: режим 24V не поддерживается")
		}
	case VendorSNR:
		switch m {
		case "off":
			return "no power inline enable", nil
		case "poe+":
			return "power inline enable", nil
		case "24v":
			return "", fmt.Errorf("SNR: режим 24V не поддерживается")
		}
	case VendorMikrotik:
		switch m {
		case "off":
			return "/interface ethernet poe set %s poe-out=off", nil
		case "poe+":
			return "/interface ethernet poe set %s poe-out=auto-on", nil
		case "24v":
			return "", fmt.Errorf("MikroTik: режим 24V не поддерживается (используйте poe+ / off)")
		}
	case VendorCisco:
		switch m {
		case "off":
			return "power inline never", nil
		case "poe+":
			return "power inline auto", nil
		case "24v":
			return "", fmt.Errorf("Cisco: режим 24V не поддерживается")
		}
	case VendorAruba:
		switch m {
		case "off":
			return "no power-over-ethernet", nil
		case "poe+":
			return "power-over-ethernet", nil
		case "24v":
			return "", fmt.Errorf("Aruba: режим 24V не поддерживается")
		}
	case VendorZyxel:
		switch m {
		case "off":
			return "poe mode off", nil
		case "poe+":
			return "poe mode auto", nil
		case "24v":
			return "", fmt.Errorf("Zyxel: режим 24V не поддерживается")
		}
	case VendorHuawei:
		switch m {
		case "off":
			return "undo poe enable", nil
		case "poe+":
			return "poe enable", nil
		case "24v":
			return "", fmt.Errorf("Huawei: режим 24V не поддерживается")
		}
	case VendorHP:
		switch m {
		case "off":
			return "no power-over-ethernet", nil
		case "poe+":
			return "power-over-ethernet", nil
		case "24v":
			return "", fmt.Errorf("HP ProCurve: режим 24V не поддерживается")
		}
	case VendorTPLink:
		switch m {
		case "off":
			return "no power inline supply", nil
		case "poe+":
			return "power inline supply", nil
		case "24v":
			return "", fmt.Errorf("TP-Link: режим 24V не поддерживается")
		}
	case VendorDLink:
		switch m {
		case "off":
			return "no power inline enable", nil
		case "poe+":
			return "power inline enable", nil
		case "24v":
			return "", fmt.Errorf("D-Link: режим 24V не поддерживается")
		}
	case VendorDahua, VendorHikvision, VendorHiWatch, VendorTrassir:
		switch m {
		case "off":
			return "poe disable", nil
		case "poe+":
			return "poe enable", nil
		case "24v":
			return "", fmt.Errorf("%s: режим 24V не поддерживается", v)
		}
	default:
		return UbiquitiPoEOpmodeCLI(m)
	}
	return "", fmt.Errorf("poe_mode %q", m)
}

// IsolateCLI — изоляция порта.
// SNR: группа netlynx (создаётся при необходимости отдельными шагами снаружи).
func IsolateCLI(v Vendor, on bool) []string {
	switch v {
	case VendorEltex:
		if on {
			return []string{"switchport protected-port"}
		}
		return []string{"no switchport protected-port"}
	case VendorSNR:
		return nil
	case VendorCisco:
		if on {
			return []string{"switchport protected"}
		}
		return []string{"no switchport protected"}
	case VendorHuawei:
		if on {
			return []string{"port-isolate enable"}
		}
		return []string{"undo port-isolate"}
	case VendorAruba, VendorZyxel, VendorMikrotik, VendorHP, VendorTPLink, VendorDLink,
		VendorDahua, VendorHikvision, VendorHiWatch, VendorTrassir:
		return nil
	default:
		return []string{UbiquitiIsolateCLI(on)}
	}
}

func SNRIsolateSteps(iface string, on bool) []string {
	iface = strings.TrimSpace(iface)
	name := iface
	low := strings.ToLower(name)
	if strings.HasPrefix(low, "interface ") {
		name = strings.TrimSpace(name[len("interface "):])
	}
	if on {
		return []string{
			"isolate-port group netlynx",
			"isolate-port group netlynx switchport interface " + name,
			"isolate-port apply l2",
		}
	}
	return []string{
		"no isolate-port group netlynx switchport interface " + name,
	}
}

// FlowControlCLI
func FlowControlCLI(v Vendor, on bool) string {
	switch v {
	case VendorEltex:
		if on {
			return "flowcontrol mode on"
		}
		return "flowcontrol mode off"
	case VendorSNR:
		if on {
			return "flow control"
		}
		return "no flow control"
	case VendorCisco, VendorAruba, VendorZyxel, VendorHP, VendorTPLink, VendorDLink,
		VendorDahua, VendorHikvision, VendorHiWatch, VendorTrassir:
		if on {
			return "flowcontrol receive on"
		}
		return "flowcontrol receive off"
	case VendorHuawei:
		if on {
			return "flow-control"
		}
		return "undo flow-control"
	default:
		return UbiquitiFlowControlCLI(on)
	}
}

// DHCPTrustCLI
func DHCPTrustCLI(on bool) string {
	return UbiquitiDHCPTrustCLI(on)
}

// EdgePortMode: auto | enable | disable
type EdgePortMode string

const (
	EdgeAuto    EdgePortMode = "auto"
	EdgeEnable  EdgePortMode = "enable"
	EdgeDisable EdgePortMode = "disable"
)

func NormalizeEdgePort(raw string) (EdgePortMode, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	switch s {
	case "auto", "":
		return EdgeAuto, nil
	case "enable", "on", "edge", "edgeport", "portfast":
		return EdgeEnable, nil
	case "disable", "off", "no":
		return EdgeDisable, nil
	default:
		return "", fmt.Errorf("неизвестный edge_port %q (auto|enable|disable)", raw)
	}
}

// STPChange — частичное изменение STP на порту.
type STPChange struct {
	Enabled      *bool
	EdgePort     *string // auto|enable|disable
	PortPriority *int    // 0..240, обычно кратно 16
	PathCost     *int    // 0 = auto
}

// STPCLILines — команды STP в режиме interface.
func STPCLILines(v Vendor, st STPChange) ([]string, error) {
	var out []string
	if st.Enabled != nil {
		switch v {
		case VendorEltex:
			if *st.Enabled {
				out = append(out, "no spanning-tree disable")
			} else {
				out = append(out, "spanning-tree disable")
			}
		case VendorSNR:
			if *st.Enabled {
				out = append(out, "spanning-tree")
			} else {
				out = append(out, "no spanning-tree")
			}
		case VendorCisco, VendorAruba, VendorZyxel, VendorHP, VendorTPLink, VendorDLink,
			VendorDahua, VendorHikvision, VendorHiWatch, VendorTrassir:
			if *st.Enabled {
				out = append(out, "spanning-tree portfast")
			} else {
				out = append(out, "no spanning-tree portfast")
			}
		case VendorHuawei:
			if *st.Enabled {
				out = append(out, "stp enable")
			} else {
				out = append(out, "stp disable")
			}
		default:
			if *st.Enabled {
				out = append(out, "spanning-tree port mode")
			} else {
				out = append(out, "no spanning-tree port mode")
			}
		}
	}
	if st.EdgePort != nil {
		ep, err := NormalizeEdgePort(*st.EdgePort)
		if err != nil {
			return nil, err
		}
		switch v {
		case VendorEltex, VendorSNR, VendorCisco, VendorAruba, VendorZyxel,
			VendorHP, VendorTPLink, VendorDLink,
			VendorDahua, VendorHikvision, VendorHiWatch, VendorTrassir:
			switch ep {
			case EdgeAuto:
				out = append(out, "spanning-tree portfast auto")
			case EdgeEnable:
				out = append(out, "spanning-tree portfast")
			case EdgeDisable:
				out = append(out, "no spanning-tree portfast")
			}
		case VendorHuawei:
			switch ep {
			case EdgeAuto, EdgeEnable:
				out = append(out, "stp edged-port enable")
			case EdgeDisable:
				out = append(out, "stp edged-port disable")
			}
		default:
			switch ep {
			case EdgeAuto:
				out = append(out, "spanning-tree edgeport auto")
			case EdgeEnable:
				out = append(out, "spanning-tree edgeport")
			case EdgeDisable:
				out = append(out, "no spanning-tree edgeport")
			}
		}
	}
	if st.PortPriority != nil {
		p := *st.PortPriority
		if p < 0 || p > 240 {
			return nil, fmt.Errorf("port_priority %d вне 0..240", p)
		}
		if v == VendorHuawei {
			out = append(out, fmt.Sprintf("stp priority %d", p))
		} else {
			out = append(out, fmt.Sprintf("spanning-tree port-priority %d", p))
		}
	}
	if st.PathCost != nil {
		c := *st.PathCost
		if c < 0 {
			return nil, fmt.Errorf("path_cost %d", c)
		}
		if v == VendorHuawei {
			if c == 0 {
				out = append(out, "undo stp cost")
			} else {
				out = append(out, fmt.Sprintf("stp cost %d", c))
			}
		} else if c == 0 {
			out = append(out, "no spanning-tree cost")
		} else {
			out = append(out, fmt.Sprintf("spanning-tree cost %d", c))
		}
	}
	return out, nil
}

// ShowInterfaceConfigCmd — команда чтения конфига порта.
func ShowInterfaceConfigCmd(v Vendor, iface string) string {
	iface = strings.TrimSpace(iface)
	switch v {
	case VendorEltex:
		return "show running-config interfaces " + iface
	case VendorSNR, VendorCisco, VendorAruba, VendorZyxel, VendorHP, VendorTPLink, VendorDLink,
		VendorDahua, VendorHikvision, VendorHiWatch, VendorTrassir:
		return "show running-config interface " + iface
	case VendorHuawei:
		return "display current-configuration interface " + iface
	default:
		return "show running-config interface " + iface
	}
}

// AdminUpCLI — shutdown / no shutdown (Huawei: undo shutdown).
func AdminUpCLI(v Vendor, up bool) string {
	if v == VendorHuawei {
		if up {
			return "undo shutdown"
		}
		return "shutdown"
	}
	if up {
		return "no shutdown"
	}
	return "shutdown"
}

// ClearDescriptionCLI
func ClearDescriptionCLI(v Vendor) string {
	if v == VendorHuawei {
		return "undo description"
	}
	return "no description"
}
