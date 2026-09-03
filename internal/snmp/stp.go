package snmp

import (
	"encoding/hex"
	"fmt"

	"github.com/gosnmp/gosnmp"
)

const (
	oidDot1dStpTopChanges     = "1.3.6.1.2.1.17.2.1.4.0"
	oidDot1dStpDesignatedRoot = "1.3.6.1.2.1.17.2.1.5.0"
	oidDot1dStpRootPort       = "1.3.6.1.2.1.17.2.1.7.0"
)

type BridgeSTP struct {
	TopChanges     int64
	DesignatedRoot string
	RootPort       int
}

// ReadBridgeSTP читает scalar BRIDGE-MIB STP (dot1dStp).
func ReadBridgeSTP(g *gosnmp.GoSNMP) (*BridgeSTP, error) {
	pdus, err := g.Get([]string{oidDot1dStpTopChanges, oidDot1dStpDesignatedRoot, oidDot1dStpRootPort})
	if err != nil {
		return nil, err
	}
	out := &BridgeSTP{RootPort: -1}
	gotChanges := false
	for _, v := range pdus.Variables {
		switch normalizeOID(v.Name) {
		case oidDot1dStpTopChanges:
			n, ok := pduCounter64(v)
			if !ok {
				return nil, fmt.Errorf("dot1dStpTopChanges: unexpected type")
			}
			out.TopChanges = n
			gotChanges = true
		case oidDot1dStpDesignatedRoot:
			out.DesignatedRoot = formatBridgeID(v)
		case oidDot1dStpRootPort:
			if n, ok := pduCounter64(v); ok {
				out.RootPort = int(n)
			}
		}
	}
	if !gotChanges {
		return nil, fmt.Errorf("bridge STP MIB недоступен")
	}
	return out, nil
}

func pduCounter64(p gosnmp.SnmpPDU) (int64, bool) {
	bi := gosnmp.ToBigInt(p.Value)
	if bi == nil {
		return 0, false
	}
	return bi.Int64(), true
}

func formatBridgeID(p gosnmp.SnmpPDU) string {
	switch v := p.Value.(type) {
	case []byte:
		if len(v) == 0 {
			return ""
		}
		return hex.EncodeToString(v)
	case string:
		return v
	default:
		return pduString(p)
	}
}
