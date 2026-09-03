package snmp

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gosnmp/gosnmp"
)

// IfRow снимок строки IF-MIB + HC-счётчики (ifXTable).
type IfRow struct {
	IfIndex   int
	IfDescr   string
	IfAlias   string
	IfName    string
	IfType    int64
	Admin     int
	Oper      int
	IfSpeed   int64 // бит/с по RFC (может быть устаревшим для >1G)
	HighSpeed int64 // Мбит/с
	HCIn      int64
	HCOut     int64
}

// BitsPerSecond номинальная скорость линка для утилизации.
func (r IfRow) BitsPerSecond() int64 {
	if r.HighSpeed > 0 {
		return r.HighSpeed * 1_000_000
	}
	if r.IfSpeed > 0 {
		return r.IfSpeed
	}
	return 0
}

const (
	baseIfDescr   = "1.3.6.1.2.1.2.2.1.2"
	baseIfType    = "1.3.6.1.2.1.2.2.1.3"
	baseIfSpeed   = "1.3.6.1.2.1.2.2.1.5"
	baseIfAdmin   = "1.3.6.1.2.1.2.2.1.7"
	baseIfOper    = "1.3.6.1.2.1.2.2.1.8"
	baseIfName    = "1.3.6.1.2.1.31.1.1.1.1"
	baseIfAlias   = "1.3.6.1.2.1.31.1.1.1.18"
	baseIfHCIn    = "1.3.6.1.2.1.31.1.1.1.6"
	baseIfHCOut   = "1.3.6.1.2.1.31.1.1.1.10"
	baseIfHighSpd = "1.3.6.1.2.1.31.1.1.1.15"
)

// WalkIFMIB обходит колонки IF-MIB / ifXTable.
// Для SNMPv1 используем Walk (GETNEXT), для v2c/v3 — BulkWalk.
func WalkIFMIB(g *gosnmp.GoSNMP) (map[int]IfRow, error) {
	m := make(map[int]IfRow)
	walkers := []struct {
		base string
		fn   func(*IfRow, gosnmp.SnmpPDU) error
	}{
		{baseIfDescr, func(r *IfRow, p gosnmp.SnmpPDU) error {
			r.IfDescr = pduString(p)
			return nil
		}},
		{baseIfType, func(r *IfRow, p gosnmp.SnmpPDU) error {
			r.IfType = pduInt64(p)
			return nil
		}},
		{baseIfSpeed, func(r *IfRow, p gosnmp.SnmpPDU) error {
			r.IfSpeed = pduInt64(p)
			return nil
		}},
		{baseIfAdmin, func(r *IfRow, p gosnmp.SnmpPDU) error {
			r.Admin = int(pduInt64(p))
			return nil
		}},
		{baseIfOper, func(r *IfRow, p gosnmp.SnmpPDU) error {
			r.Oper = int(pduInt64(p))
			return nil
		}},
		{baseIfName, func(r *IfRow, p gosnmp.SnmpPDU) error {
			r.IfName = pduString(p)
			return nil
		}},
		{baseIfAlias, func(r *IfRow, p gosnmp.SnmpPDU) error {
			r.IfAlias = pduString(p)
			return nil
		}},
		{baseIfHCIn, func(r *IfRow, p gosnmp.SnmpPDU) error {
			r.HCIn = pduInt64(p)
			return nil
		}},
		{baseIfHCOut, func(r *IfRow, p gosnmp.SnmpPDU) error {
			r.HCOut = pduInt64(p)
			return nil
		}},
		{baseIfHighSpd, func(r *IfRow, p gosnmp.SnmpPDU) error {
			r.HighSpeed = pduInt64(p)
			return nil
		}},
	}
	for _, w := range walkers {
		walkFn := g.BulkWalk
		walkName := "bulkwalk"
		if g.Version == gosnmp.Version1 {
			walkFn = g.Walk
			walkName = "walk"
		}
		err := walkFn(w.base, func(pdu gosnmp.SnmpPDU) error {
			idx, err := parseIfIndexSuffix(pdu.Name, w.base)
			if err != nil {
				return nil // пропускаем нестандартные OID
			}
			r := m[idx]
			r.IfIndex = idx
			if err := w.fn(&r, pdu); err != nil {
				return err
			}
			m[idx] = r
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("%s %s: %w", walkName, w.base, err)
		}
	}
	return m, nil
}

func parseIfIndexSuffix(oid, base string) (int, error) {
	oid = strings.TrimPrefix(oid, ".")
	base = strings.TrimPrefix(base, ".")
	if !strings.HasPrefix(oid, base+".") {
		return 0, fmt.Errorf("oid mismatch")
	}
	suf := strings.TrimPrefix(oid, base+".")
	i, err := strconv.Atoi(suf)
	if err != nil {
		return 0, err
	}
	return i, nil
}

func pduInt64(p gosnmp.SnmpPDU) int64 {
	bi := gosnmp.ToBigInt(p.Value)
	if bi == nil {
		return 0
	}
	return bi.Int64()
}
