package snmp

import (
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/gosnmp/gosnmp"
)

// Cisco CDP-MIB (CISCO-CDP-MIB)
const (
	baseCdpCacheDeviceId   = "1.3.6.1.4.1.9.9.23.1.2.1.1.6"
	baseCdpCacheDevicePort = "1.3.6.1.4.1.9.9.23.1.2.1.1.7"
	baseCdpCacheAddress    = "1.3.6.1.4.1.9.9.23.1.2.1.1.4"
)

// WalkCDPNeighbors читает cdpCacheTable. Индекс: ifIndex.deviceIndex.
// На устройствах без CDP walk обычно возвращает ошибку — вызывающий не должен чистить БД.
func WalkCDPNeighbors(g *gosnmp.GoSNMP) ([]NeighborInfo, error) {
	type remKey struct {
		ifIndex  int
		remIndex int
	}
	byKey := make(map[remKey]*NeighborInfo)

	parseIdx := func(oid, base string) (ifIndex, remIndex int, ok bool) {
		suf := lldpSuffixAfterBase(oid, base)
		parts := strings.Split(strings.Trim(suf, "."), ".")
		if len(parts) < 2 {
			return 0, 0, false
		}
		ifi, err1 := strconv.Atoi(parts[0])
		ri, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil || ifi <= 0 {
			return 0, 0, false
		}
		if ri <= 0 {
			ri = 1
		}
		return ifi, ri, true
	}

	ensure := func(ifi, ri int) *NeighborInfo {
		k := remKey{ifIndex: ifi, remIndex: ri}
		if byKey[k] == nil {
			byKey[k] = &NeighborInfo{IfIndex: ifi, RemIndex: ri}
		}
		return byKey[k]
	}

	if err := g.BulkWalk(baseCdpCacheDeviceId, func(pdu gosnmp.SnmpPDU) error {
		ifi, ri, ok := parseIdx(pdu.Name, baseCdpCacheDeviceId)
		if !ok {
			return nil
		}
		ensure(ifi, ri).RemoteSysName = pduString(pdu)
		return nil
	}); err != nil {
		return nil, err
	}

	_ = g.BulkWalk(baseCdpCacheDevicePort, func(pdu gosnmp.SnmpPDU) error {
		ifi, ri, ok := parseIdx(pdu.Name, baseCdpCacheDevicePort)
		if !ok {
			return nil
		}
		ensure(ifi, ri).RemotePortID = pduString(pdu)
		return nil
	})

	_ = g.BulkWalk(baseCdpCacheAddress, func(pdu gosnmp.SnmpPDU) error {
		ifi, ri, ok := parseIdx(pdu.Name, baseCdpCacheAddress)
		if !ok {
			return nil
		}
		if addr := cdpCacheAddressString(pdu); addr != "" {
			ensure(ifi, ri).RemoteMgmtAddr = addr
			// Не пишем IP в RemoteChassisID через FormatMAC — это не MAC
			// (раньше получалось 01:c0:a8:… из type+IPv4).
		}
		return nil
	})

	out := make([]NeighborInfo, 0, len(byKey))
	for _, n := range byKey {
		if n == nil {
			continue
		}
		if strings.TrimSpace(n.RemoteSysName+n.RemotePortID+n.RemoteChassisID+n.RemoteMgmtAddr) == "" {
			continue
		}
		out = append(out, *n)
	}
	return out, nil
}

// cdpCacheAddress: OCTET STRING — часто type(1) + IPv4(4) или просто IPv4.
func cdpCacheAddressString(pdu gosnmp.SnmpPDU) string {
	switch v := pdu.Value.(type) {
	case string:
		b := []byte(v)
		return decodeCDPAddress(b)
	case []byte:
		return decodeCDPAddress(v)
	default:
		s := strings.TrimSpace(pduString(pdu))
		if net.ParseIP(s) != nil {
			return s
		}
		return ""
	}
}

func decodeCDPAddress(b []byte) string {
	if len(b) == 4 {
		return net.IP(b).String()
	}
	if len(b) >= 5 && b[0] == 1 {
		return net.IP(b[1:5]).String()
	}
	if len(b) >= 6 && binary.BigEndian.Uint16(b[0:2]) == 1 && len(b) >= 6 {
		return net.IP(b[2:6]).String()
	}
	if ip := net.ParseIP(string(b)); ip != nil {
		return ip.String()
	}
	if len(b) == 6 {
		return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", b[0], b[1], b[2], b[3], b[4], b[5])
	}
	return ""
}
