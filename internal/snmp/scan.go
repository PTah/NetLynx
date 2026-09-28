package snmp

import (
	"fmt"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"
)

// ProbeSysOptions — параметры быстрой SNMP-пробы для subnet scan.
type ProbeSysOptions struct {
	Host      string
	Version   string // v1 | v2c
	Community string
	Timeout   time.Duration
	Retries   int
}

// ProbeSys делает Connect + SysGet и закрывает сессию.
// Возвращает sysName/sysDescr при успехе; иначе ошибку (таймаут, отказ community и т.п.).
func ProbeSys(opt ProbeSysOptions) (sysName, sysDescr string, err error) {
	host := strings.TrimSpace(opt.Host)
	if host == "" {
		return "", "", fmt.Errorf("пустой host")
	}
	ver := strings.TrimSpace(strings.ToLower(opt.Version))
	if ver == "" {
		ver = "v2c"
	}
	comm := strings.TrimSpace(opt.Community)
	if comm == "" {
		return "", "", fmt.Errorf("пустой community")
	}
	timeout := opt.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	retries := opt.Retries
	if retries < 0 {
		retries = 0
	}
	g := &gosnmp.GoSNMP{
		Target:    host,
		Port:      161,
		Timeout:   timeout,
		Retries:   retries,
		Community: comm,
	}
	switch ver {
	case "v1":
		g.Version = gosnmp.Version1
	case "v2c":
		g.Version = gosnmp.Version2c
	default:
		return "", "", fmt.Errorf("скан поддерживает только SNMP v1/v2c, не %q", ver)
	}
	if err := g.Connect(); err != nil {
		return "", "", err
	}
	defer g.Conn.Close()
	return SysGet(g)
}
