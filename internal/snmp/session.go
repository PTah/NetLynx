package snmp

import (
	"fmt"
	"strings"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
	"github.com/gosnmp/gosnmp"
)

// NewGoSNMP создаёт клиента для одного опроса (Connect вызывает вызывающий).
func NewGoSNMP(d store.PollDevice) (*gosnmp.GoSNMP, error) {
	g := &gosnmp.GoSNMP{
		Target:  d.Host,
		Port:    161,
		Timeout: 5 * time.Second,
		Retries: 1,
	}
	switch d.SNMPVersion {
	case "v1":
		g.Version = gosnmp.Version1
		if d.Community == nil || strings.TrimSpace(*d.Community) == "" {
			return nil, fmt.Errorf("SNMP v1: пустой community")
		}
		g.Community = strings.TrimSpace(*d.Community)
	case "v2c":
		g.Version = gosnmp.Version2c
		if d.Community == nil || strings.TrimSpace(*d.Community) == "" {
			return nil, fmt.Errorf("SNMP v2c: пустой community")
		}
		g.Community = strings.TrimSpace(*d.Community)
	case "v3":
		g.Version = gosnmp.Version3
		if d.V3User == nil || strings.TrimSpace(*d.V3User) == "" {
			return nil, fmt.Errorf("SNMP v3: пустой user")
		}
		authRaw := strings.ToUpper(strings.TrimSpace(deref(d.V3AuthProtocol)))
		if authRaw == "" {
			authRaw = "SHA"
		}
		authProto, ok := parseAuthProto(authRaw)
		if !ok {
			return nil, fmt.Errorf("SNMP v3: неподдерживаемый auth protocol %q", authRaw)
		}
		authPass := strings.TrimSpace(deref(d.V3AuthPass))
		if len(authPass) < 8 {
			return nil, fmt.Errorf("SNMP v3: auth passphrase должна быть >= 8 символов")
		}
		privRaw := strings.ToUpper(strings.TrimSpace(deref(d.V3PrivProtocol)))
		if privRaw == "" {
			privRaw = "AES"
		}
		privProto, ok := parsePrivProto(privRaw)
		if !ok {
			return nil, fmt.Errorf("SNMP v3: неподдерживаемый privacy protocol %q", privRaw)
		}
		privPass := strings.TrimSpace(deref(d.V3PrivPass))
		if privProto != gosnmp.NoPriv && len(privPass) < 8 {
			return nil, fmt.Errorf("SNMP v3: privacy passphrase должна быть >= 8 символов (или protocol NONE)")
		}
		flags := gosnmp.AuthPriv
		if privProto == gosnmp.NoPriv {
			flags = gosnmp.AuthNoPriv
		}
		g.SecurityModel = gosnmp.UserSecurityModel
		g.MsgFlags = flags
		usp := &gosnmp.UsmSecurityParameters{
			UserName:                 strings.TrimSpace(*d.V3User),
			AuthenticationProtocol:   authProto,
			AuthenticationPassphrase: authPass,
			PrivacyProtocol:          privProto,
			PrivacyPassphrase:        privPass,
		}
		if d.V3EngineID != nil && strings.TrimSpace(*d.V3EngineID) != "" {
			usp.AuthoritativeEngineID = strings.TrimSpace(*d.V3EngineID)
		}
		g.SecurityParameters = usp
	default:
		return nil, fmt.Errorf("неизвестная версия SNMP: %q", d.SNMPVersion)
	}
	return g, nil
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func parseAuthProto(s string) (gosnmp.SnmpV3AuthProtocol, bool) {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "", "SHA":
		return gosnmp.SHA, true
	case "MD5":
		return gosnmp.MD5, true
	case "SHA224":
		return gosnmp.SHA224, true
	case "SHA256":
		return gosnmp.SHA256, true
	case "SHA384":
		return gosnmp.SHA384, true
	case "SHA512":
		return gosnmp.SHA512, true
	default:
		return gosnmp.NoAuth, false
	}
}

func parsePrivProto(s string) (gosnmp.SnmpV3PrivProtocol, bool) {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "", "AES":
		return gosnmp.AES, true
	case "AES128":
		return gosnmp.AES, true
	case "AES192":
		return gosnmp.AES192, true
	case "AES256":
		return gosnmp.AES256, true
	case "DES":
		return gosnmp.DES, true
	case "NONE":
		return gosnmp.NoPriv, true
	default:
		return gosnmp.NoPriv, false
	}
}
