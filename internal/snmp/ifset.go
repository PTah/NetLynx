package snmp

import (
	"fmt"
	"strings"

	"github.com/gosnmp/gosnmp"
)

const (
	oidIfAdminStatusPrefix = "1.3.6.1.2.1.2.2.1.7"
	// ifAlias (IF-MIB) — обычно writable; на EdgeSwitch часто read-only community → SET «молчит».
	oidIfAliasPrefix = "1.3.6.1.2.1.31.1.1.1.18"
)

func checkSetPacket(pkt *gosnmp.SnmpPacket, err error) error {
	if err != nil {
		return err
	}
	if pkt == nil {
		return fmt.Errorf("snmp set: пустой ответ")
	}
	if pkt.Error != gosnmp.NoError {
		return fmt.Errorf("snmp set: %s (index %d)", pkt.Error.String(), pkt.ErrorIndex)
	}
	return nil
}

// SetIfAdminStatus устанавливает ifAdminStatus (1=up, 2=down, 3=testing).
func SetIfAdminStatus(g *gosnmp.GoSNMP, ifIndex, status int) error {
	if ifIndex <= 0 {
		return fmt.Errorf("invalid ifIndex %d", ifIndex)
	}
	if status != 1 && status != 2 {
		return fmt.Errorf("ifAdminStatus: только 1 (up) или 2 (down)")
	}
	oid := fmt.Sprintf("%s.%d", oidIfAdminStatusPrefix, ifIndex)
	pkt, err := g.Set([]gosnmp.SnmpPDU{{
		Name:  oid,
		Type:  gosnmp.Integer,
		Value: status,
	}})
	if err := checkSetPacket(pkt, err); err != nil {
		return err
	}
	got, gerr := GetIfAdminStatus(g, ifIndex)
	if gerr != nil {
		return fmt.Errorf("snmp set admin ok, но verify: %w", gerr)
	}
	if got != status {
		return fmt.Errorf("snmp set admin: записали %d, прочитали %d (community без write?)", status, got)
	}
	return nil
}

// GetIfAdminStatus читает ifAdminStatus.
func GetIfAdminStatus(g *gosnmp.GoSNMP, ifIndex int) (int, error) {
	oid := fmt.Sprintf("%s.%d", oidIfAdminStatusPrefix, ifIndex)
	pkt, err := g.Get([]string{oid})
	if err != nil {
		return 0, err
	}
	if pkt == nil || len(pkt.Variables) == 0 {
		return 0, fmt.Errorf("нет ответа")
	}
	return int(pduInt64(pkt.Variables[0])), nil
}

// SetIfAlias пишет ifAlias и проверяет чтением обратно.
func SetIfAlias(g *gosnmp.GoSNMP, ifIndex int, alias string) error {
	if ifIndex <= 0 {
		return fmt.Errorf("invalid ifIndex %d", ifIndex)
	}
	if len(alias) > 64 {
		alias = alias[:64]
	}
	oid := fmt.Sprintf("%s.%d", oidIfAliasPrefix, ifIndex)
	pkt, err := g.Set([]gosnmp.SnmpPDU{{
		Name:  oid,
		Type:  gosnmp.OctetString,
		Value: alias,
	}})
	if err := checkSetPacket(pkt, err); err != nil {
		return err
	}
	got, gerr := GetIfAlias(g, ifIndex)
	if gerr != nil {
		return fmt.Errorf("snmp set ifAlias ok, но verify: %w", gerr)
	}
	if strings.TrimSpace(got) != strings.TrimSpace(alias) {
		return fmt.Errorf("snmp set ifAlias не применился (ожидали %q, прочитали %q; нужен write community или SSH)", alias, got)
	}
	return nil
}

// GetIfAlias читает ifAlias.
func GetIfAlias(g *gosnmp.GoSNMP, ifIndex int) (string, error) {
	oid := fmt.Sprintf("%s.%d", oidIfAliasPrefix, ifIndex)
	pkt, err := g.Get([]string{oid})
	if err != nil {
		return "", err
	}
	if pkt == nil || len(pkt.Variables) == 0 {
		return "", fmt.Errorf("нет ответа")
	}
	return strings.TrimSpace(pduString(pkt.Variables[0])), nil
}
