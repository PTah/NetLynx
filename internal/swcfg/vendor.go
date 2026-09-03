package swcfg

import (
	"strings"
)

type Vendor string

const (
	VendorAuto      Vendor = "auto"
	VendorUbiquiti  Vendor = "ubiquiti"
	VendorEltex     Vendor = "eltex"
	VendorSNR       Vendor = "snr"
	VendorMikrotik  Vendor = "mikrotik"
	VendorCisco     Vendor = "cisco"
	VendorAruba     Vendor = "aruba"
	VendorZyxel     Vendor = "zyxel"
	VendorHuawei    Vendor = "huawei"
	VendorHP        Vendor = "hp" // ProCurve / older HPE switch OS
	VendorTPLink    Vendor = "tplink"
	VendorDLink     Vendor = "dlink"
	VendorDahua     Vendor = "dahua"
	VendorHikvision Vendor = "hikvision"
	VendorHiWatch   Vendor = "hiwatch"
	VendorTrassir   Vendor = "trassir"
)

var knownVendors = map[string]Vendor{
	"ubiquiti": VendorUbiquiti, "eltex": VendorEltex, "snr": VendorSNR,
	"mikrotik": VendorMikrotik, "cisco": VendorCisco, "aruba": VendorAruba,
	"zyxel": VendorZyxel, "huawei": VendorHuawei, "hp": VendorHP,
	"procurve": VendorHP, "tplink": VendorTPLink, "tp-link": VendorTPLink,
	"dlink": VendorDLink, "d-link": VendorDLink, "dahua": VendorDahua,
	"hikvision": VendorHikvision, "hiwatch": VendorHiWatch, "trassir": VendorTrassir,
}

func DetectVendor(explicit, sysDescr, name string) Vendor {
	if v, ok := knownVendors[strings.ToLower(strings.TrimSpace(explicit))]; ok {
		return v
	}
	blob := strings.ToLower(sysDescr + " " + name)
	switch {
	case strings.Contains(blob, "mikrotik") || strings.Contains(blob, "routeros") ||
		strings.Contains(blob, "ccr") || strings.Contains(blob, "crs") || strings.Contains(blob, "css326") ||
		strings.Contains(blob, "routerboard"):
		return VendorMikrotik
	case strings.Contains(blob, "huawei") || strings.Contains(blob, "vrp") || strings.Contains(blob, "quidway"):
		return VendorHuawei
	case strings.Contains(blob, "aruba") || strings.Contains(blob, "aos-cx") ||
		strings.Contains(blob, "hpe networking") || strings.Contains(blob, "hp networking"):
		return VendorAruba
	case strings.Contains(blob, "procurve") || strings.Contains(blob, "hewlett-packard") ||
		strings.Contains(blob, "hewlett packard") ||
		(strings.Contains(blob, "hp ") && (strings.Contains(blob, "switch") || strings.Contains(blob, "j9") || strings.Contains(blob, "j8"))):
		return VendorHP
	case strings.Contains(blob, "zyxel"):
		return VendorZyxel
	case strings.Contains(blob, "cisco") || strings.Contains(blob, "nx-os") || strings.Contains(blob, "ios-xe") ||
		strings.Contains(blob, "catalyst") || strings.Contains(blob, "nexus") ||
		strings.Contains(blob, "cisco ios"):
		return VendorCisco
	case strings.Contains(blob, "tp-link") || strings.Contains(blob, "tplink") ||
		strings.Contains(blob, "jetstream") || strings.Contains(blob, "tl-sg") ||
		strings.Contains(blob, "t1500") || strings.Contains(blob, "t1600") || strings.Contains(blob, "t2500") ||
		strings.Contains(blob, "t2600") || strings.Contains(blob, "sg3"):
		return VendorTPLink
	case strings.Contains(blob, "d-link") || strings.Contains(blob, "dlink") ||
		strings.Contains(blob, "dgs-") || strings.Contains(blob, "des-") || strings.Contains(blob, "dxs-"):
		return VendorDLink
	case looksLikeVideoLANSwitch(blob, "dahua") || strings.Contains(blob, "dh-pfs") || strings.Contains(blob, "pfs42"):
		return VendorDahua
	case looksLikeVideoLANSwitch(blob, "hiwatch") || looksLikeVideoLANSwitch(blob, "hi-watch") ||
		(strings.Contains(blob, "hiwatch") && strings.Contains(blob, "ds-3e")):
		return VendorHiWatch
	case looksLikeVideoLANSwitch(blob, "hikvision") ||
		(strings.Contains(blob, "ds-3e") && !strings.Contains(blob, "hiwatch") && !strings.Contains(blob, "hi-watch")):
		return VendorHikvision
	case looksLikeVideoLANSwitch(blob, "trassir"):
		return VendorTrassir
	case strings.Contains(blob, "eltex") || strings.Contains(blob, " mes"):
		return VendorEltex
	case strings.Contains(blob, "snr") || strings.Contains(blob, "nag llc"):
		return VendorSNR
	case strings.Contains(blob, "edgeswitch") || strings.Contains(blob, "ubiquiti") || strings.Contains(blob, "ubnt") || strings.Contains(blob, "unifi"):
		return VendorUbiquiti
	default:
		return VendorAuto
	}
}

// LooksLikeIPCamera — FDB/топология: камера, не свитч того же бренда.
func LooksLikeIPCamera(sysDescr, name string) bool {
	blob := strings.ToLower(sysDescr + " " + name)
	if LooksLikeVideoLANSwitch(sysDescr, name) {
		return false
	}
	for _, h := range []string{
		"ip camera", "ipcam", "ipc-", "ipc ", "webcam", "network camera",
		"ds-2cd", "ds-2de", "ds-2df", "dh-ipc", "hikvision ipc",
		"hiwatch ipc", "camera",
	} {
		if strings.Contains(blob, h) {
			return true
		}
	}
	// Бренд камеры без маркеров свитча → скорее камера (старое поведение для hikvision в FDB).
	if (strings.Contains(blob, "hikvision") || strings.Contains(blob, "dahua") ||
		strings.Contains(blob, "hiwatch") || strings.Contains(blob, "trassir")) &&
		!strings.Contains(blob, "switch") && !strings.Contains(blob, "pfs") &&
		!strings.Contains(blob, "ds-3e") {
		return true
	}
	return false
}

// LooksLikeVideoLANSwitch — управляемый свитч video-LAN бренда (не камера).
func LooksLikeVideoLANSwitch(sysDescr, name string) bool {
	return looksLikeVideoLANSwitch(strings.ToLower(sysDescr+" "+name), "")
}

func looksLikeVideoLANSwitch(blob, brand string) bool {
	if brand != "" && !strings.Contains(blob, brand) {
		return false
	}
	if strings.Contains(blob, "ipc") || strings.Contains(blob, "ipcam") ||
		strings.Contains(blob, "ip camera") || strings.Contains(blob, "ds-2cd") ||
		strings.Contains(blob, "ds-2de") || strings.Contains(blob, "dh-ipc") {
		return false
	}
	modelHints := []string{
		"dh-pfs", "pfs30", "pfs42", "pfs52", "ds-3e", "ds-3eb", "ds-3es",
		"tr-ns", "trassir switch", "managed switch", "poe switch",
		"ethernet switch", "l2 switch", "l3 switch",
	}
	for _, h := range modelHints {
		if strings.Contains(blob, h) {
			return true
		}
	}
	hasBrand := strings.Contains(blob, "dahua") || strings.Contains(blob, "hikvision") ||
		strings.Contains(blob, "hiwatch") || strings.Contains(blob, "hi-watch") ||
		strings.Contains(blob, "trassir")
	if !hasBrand && brand == "" {
		return false
	}
	return strings.Contains(blob, "switch") || strings.Contains(blob, "pfs")
}

// IsCiscoLike — IOS-like enable/configure.
func IsCiscoLike(v Vendor) bool {
	switch v {
	case VendorCisco, VendorAruba, VendorZyxel, VendorUbiquiti, VendorEltex, VendorSNR,
		VendorHP, VendorTPLink, VendorDLink, VendorDahua, VendorHikvision, VendorHiWatch, VendorTrassir:
		return true
	default:
		return false
	}
}
