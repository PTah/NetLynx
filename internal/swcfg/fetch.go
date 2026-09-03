package swcfg

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type Creds struct {
	Host       string
	Port       int
	User       string
	Password   string
	EnablePass string
	Vendor     Vendor
	SysDescr   string
	Name       string
	Timeout    time.Duration
	KnownHosts string
}

func FetchConfig(c Creds) ([]byte, error) {
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 700 * time.Millisecond)
		}
		b, err := fetchConfigOnce(c)
		if err == nil {
			return b, nil
		}
		last = err
		if !isSSHFlaky(err) {
			return nil, err
		}
	}
	return nil, last
}

func isSSHFlaky(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "connection reset") ||
		strings.Contains(s, "broken pipe") ||
		strings.Contains(s, "unexpected packet") ||
		s == "eof" ||
		strings.HasSuffix(s, ": eof") ||
		strings.Contains(s, "ssh: unexpected eof")
}

func fetchConfigOnce(c Creds) ([]byte, error) {
	host := strings.TrimSpace(c.Host)
	user := strings.TrimSpace(c.User)
	if host == "" || user == "" {
		return nil, fmt.Errorf("нет host или ssh user")
	}
	port := c.Port
	if port <= 0 {
		port = 22
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	hk, err := HostKeyCallback(c.KnownHosts)
	if err != nil {
		return nil, err
	}
	addr := fmt.Sprintf("%s:%d", host, port)
	v := DetectVendor(string(c.Vendor), c.SysDescr, c.Name)
	enable := strings.TrimSpace(c.EnablePass)
	if enable == "" {
		enable = c.Password
	}
	cfg := switchSSHConfig(user, c.Password, timeout, hk)

	if v == VendorMikrotik || DetectVendor("", c.SysDescr, c.Name) == VendorMikrotik {
		client, err := ssh.Dial("tcp", addr, cfg)
		if err != nil {
			return nil, fmt.Errorf("ssh %s: %w", host, err)
		}
		out, ferr := fetchMikrotikExport(client, timeout)
		_ = client.Close()
		if ferr != nil {
			return nil, ferr
		}
		return []byte(out), nil
	}

	if preferBusybox(c, v) {
		bb, err := ssh.Dial("tcp", addr, cfg)
		if err != nil {
			return nil, fmt.Errorf("ssh %s: %w", host, err)
		}
		out, berr := dumpBusyboxCfg(bb, timeout)
		_ = bb.Close()
		if berr == nil && looksLikeConfig(out) {
			return []byte(out), nil
		}
		if isEdgeSwitchXP(c.SysDescr, c.Name) {
			if berr != nil {
				return nil, fmt.Errorf("EdgeSwitch XP (BusyBox, нет Fastpath CLI): %w", berr)
			}
			return nil, fmt.Errorf("EdgeSwitch XP: не удалось прочитать /tmp/system.cfg")
		}
	}

	client, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return nil, fmt.Errorf("ssh %s: %w", host, err)
	}
	defer client.Close()

	var steps []string
	switch v {
	case VendorEltex:
		steps = []string{"enable", enable, "terminal datadump", "show running-config"}
	case VendorSNR:
		steps = []string{"enable", enable, "terminal length 0", "show running-config"}
	case VendorCisco, VendorAruba, VendorZyxel, VendorHP, VendorTPLink, VendorDLink,
		VendorDahua, VendorHikvision, VendorHiWatch, VendorTrassir:
		steps = []string{"enable", enable, "terminal length 0", "show running-config"}
	case VendorHuawei:
		steps = []string{"screen-length 0 temporary", "display current-configuration"}
	default:
		// Fastpath: terminal length 0 только в privileged (#). До enable пейджер не выключается.
		steps = []string{"en", enable, "terminal length 0", "show running-config"}
	}
	out, err := ciscoLikeRunning(client, timeout, steps, c.Password, enable)
	if err == nil {
		return []byte(out), nil
	}
	if looksLikeBusyboxShell(out) {
		if cfg, berr := dumpBusyboxCfg(client, timeout); berr == nil && looksLikeConfig(cfg) {
			return []byte(cfg), nil
		}
	}
	return nil, err
}

func preferBusybox(c Creds, v Vendor) bool {
	switch v {
	case VendorMikrotik, VendorCisco, VendorAruba, VendorZyxel, VendorHuawei, VendorEltex, VendorSNR,
		VendorHP, VendorTPLink, VendorDLink,
		VendorDahua, VendorHikvision, VendorHiWatch, VendorTrassir:
		return false
	}
	if v != VendorAuto && v != VendorUbiquiti {
		return false
	}
	if isEdgeSwitchXP(c.SysDescr, c.Name) {
		return true
	}
	blob := strings.ToLower(c.SysDescr + " " + c.Name)
	// Fastpath 8/16/24/48: в sysDescr есть «Linux 3.6.x», но SSH exec/cat роняет Dropbear.
	if strings.Contains(blob, "edgeswitch") {
		return false
	}
	if strings.Contains(blob, "linux") || strings.Contains(blob, "edgeos") || strings.Contains(blob, "busybox") {
		return true
	}
	return v == VendorUbiquiti || v == VendorAuto
}

func isEdgeSwitchXP(sysDescr, name string) bool {
	blob := strings.ToLower(sysDescr + " " + name)
	if strings.Contains(blob, "edgeswitch xp") || strings.Contains(blob, "es-xp") || strings.Contains(blob, "toughswitch") {
		return true
	}
	for _, tok := range []string{"5xp", "8xp", "16xp", "es-5xp", "es-8xp", "es-16xp"} {
		if strings.Contains(blob, tok) {
			return true
		}
	}
	return false
}

// IsEdgeSwitchXP — публичная обёртка (XP: нет ifAlias/Fastpath CLI для description).
func IsEdgeSwitchXP(sysDescr, name string) bool {
	return isEdgeSwitchXP(sysDescr, name)
}

func dumpBusyboxCfg(client *ssh.Client, timeout time.Duration) (string, error) {
	if s, err := tryBusyboxCat(client); err == nil && looksLikeConfig(s) {
		return s, nil
	}
	return tryBusyboxCatShell(client, timeout)
}

func tryBusyboxCat(client *ssh.Client) (string, error) {
	for _, cmd := range []string{
		"if [ -f /tmp/system.cfg ]; then cat /tmp/system.cfg; elif [ -f /tmp/running.cfg ]; then cat /tmp/running.cfg; else echo NOCFG; fi",
		"cat /tmp/system.cfg",
		"cat /tmp/running.cfg",
	} {
		sess, err := client.NewSession()
		if err != nil {
			return "", err
		}
		b, runErr := sess.CombinedOutput(cmd)
		_ = sess.Close()
		s := string(b)
		if runErr != nil {
			continue
		}
		if strings.Contains(s, "NOCFG") || looksLikeUnsupported(s) {
			continue
		}
		if looksLikeConfig(s) {
			return s, nil
		}
	}
	return "", fmt.Errorf("нет /tmp/system.cfg (exec)")
}

func tryBusyboxCatShell(client *ssh.Client, timeout time.Duration) (string, error) {
	for _, cmd := range []string{"cat /tmp/system.cfg", "cat /tmp/running.cfg"} {
		out, err := shellCommand(client, timeout, cmd)
		if err != nil {
			continue
		}
		if looksLikeConfig(out) {
			return out, nil
		}
	}
	return "", fmt.Errorf("нет /tmp/system.cfg в BusyBox-shell")
}

func shellCommand(client *ssh.Client, timeout time.Duration, cmd string) (string, error) {
	sess, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer sess.Close()
	in, err := sess.StdinPipe()
	if err != nil {
		return "", err
	}
	var outBuf, errBuf bytes.Buffer
	sess.Stdout = &outBuf
	sess.Stderr = &errBuf
	if err := sess.RequestPty("xterm", 80, 40, ssh.TerminalModes{
		ssh.ECHO:          0,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}); err != nil {
		return "", err
	}
	if err := sess.Shell(); err != nil {
		return "", err
	}
	time.Sleep(400 * time.Millisecond)
	if _, err := in.Write([]byte(cmd + "\r\n")); err != nil {
		return "", err
	}
	dumpWait := timeout
	if dumpWait < 20*time.Second {
		dumpWait = 20 * time.Second
	}
	waitQuiet(&outBuf, 800*time.Millisecond, dumpWait)
	_, _ = in.Write([]byte("exit\r\n"))
	_ = in.Close()
	_ = sess.Wait()
	return outBuf.String() + errBuf.String(), nil
}

func ciscoLikeRunning(client *ssh.Client, timeout time.Duration, steps []string, redact ...string) (string, error) {
	sess, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer sess.Close()
	in, err := sess.StdinPipe()
	if err != nil {
		return "", err
	}
	var outBuf, errBuf bytes.Buffer
	sess.Stdout = &outBuf
	sess.Stderr = &errBuf
	if err := sess.RequestPty("xterm", 80, 40, ssh.TerminalModes{
		ssh.ECHO:          0,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}); err != nil {
		return "", err
	}
	if err := sess.Shell(); err != nil {
		return "", err
	}
	time.Sleep(400 * time.Millisecond)

	var prefix []string
	showCmd := "show running-config"
	for _, s := range steps {
		low := strings.ToLower(strings.TrimSpace(s))
		if strings.HasPrefix(low, "show ") || strings.HasPrefix(low, "display ") {
			showCmd = s
			continue
		}
		prefix = append(prefix, s)
	}
	for _, s := range prefix {
		if _, err := in.Write([]byte(s + "\r\n")); err != nil {
			return "", err
		}
		time.Sleep(350 * time.Millisecond)
	}
	time.Sleep(400 * time.Millisecond)
	if _, err := in.Write([]byte(showCmd + "\r\n")); err != nil {
		return "", err
	}
	dumpWait := timeout
	if dumpWait < 90*time.Second {
		dumpWait = 90 * time.Second
	}
	waitDump(in, &outBuf, 1500*time.Millisecond, dumpWait)
	_, _ = in.Write([]byte("exit\r\n"))
	_ = in.Close()
	waitErr := sess.Wait()
	out := outBuf.String() + errBuf.String()
	if strings.TrimSpace(out) == "" {
		if waitErr != nil {
			return "", fmt.Errorf("пустой вывод CLI: %w", waitErr)
		}
		return "", fmt.Errorf("пустой вывод CLI")
	}
	if looksLikeBusyboxShell(out) {
		return out, fmt.Errorf("это BusyBox/ash, не Cisco-like CLI")
	}
	if !looksLikeConfig(out) {
		snip := compactCLIErr(out, redact...)
		return out, fmt.Errorf("вывод CLI не похож на конфиг: %s", snip)
	}
	return stripPager(out), nil
}

func waitQuiet(buf *bytes.Buffer, quiet, max time.Duration) {
	waitDump(nil, buf, quiet, max)
}

func waitDump(in io.Writer, buf *bytes.Buffer, quiet, max time.Duration) {
	if max <= 0 {
		max = 30 * time.Second
	}
	if quiet <= 0 {
		quiet = time.Second
	}
	deadline := time.Now().Add(max)
	start := time.Now()
	last := 0
	lastChange := time.Now()
	lastSpace := time.Time{}
	for time.Now().Before(deadline) {
		n := buf.Len()
		if n == 0 && time.Since(start) >= 12*time.Second {
			return
		}
		if n > last {
			last = n
			lastChange = time.Now()
		}
		if in != nil && tailHasPager(buf.String()) && time.Since(lastChange) >= 80*time.Millisecond {
			if lastSpace.IsZero() || time.Since(lastSpace) >= 80*time.Millisecond {
				_, _ = in.Write([]byte(" "))
				lastSpace = time.Now()
			}
			time.Sleep(80 * time.Millisecond)
			continue
		}
		if n > 0 && time.Since(lastChange) >= quiet && !tailHasPager(buf.String()) {
			return
		}
		time.Sleep(80 * time.Millisecond)
	}
}

func tailHasPager(s string) bool {
	if len(s) > 80 {
		s = s[len(s)-80:]
	}
	l := strings.ToLower(s)
	return strings.Contains(l, "--more--") || strings.Contains(l, "(q)uit")
}

func compactCLIErr(s string, secrets ...string) string {
	s = redactSecrets(s, secrets...)
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 240 {
		return s[:240] + "…"
	}
	if s == "" {
		return "(пусто)"
	}
	return s
}

func redactSecrets(s string, secrets ...string) string {
	for _, sec := range secrets {
		sec = strings.TrimSpace(sec)
		if sec == "" {
			continue
		}
		s = strings.ReplaceAll(s, sec, "****")
	}
	return s
}

func looksLikeBusyboxShell(s string) bool {
	l := strings.ToLower(s)
	return strings.Contains(l, "busybox") ||
		strings.Contains(l, "built-in shell (ash)") ||
		strings.Contains(l, "-sh:") ||
		(strings.Contains(l, "sw.v") && strings.Contains(l, "not found"))
}

func looksLikeUnsupported(s string) bool {
	l := strings.ToLower(s)
	return strings.Contains(l, "not found") ||
		strings.Contains(l, "no such file") ||
		strings.Contains(l, "unknown command") ||
		strings.Contains(l, "invalid input")
}

func looksLikeConfig(s string) bool {
	l := strings.ToLower(s)
	if strings.Count(s, "\n") < 3 {
		return false
	}
	return strings.Contains(l, "hostname") ||
		strings.Contains(l, "interface") ||
		strings.Contains(l, "vlan") ||
		strings.Contains(l, "aaa") ||
		strings.Contains(l, "current configuration") ||
		strings.Contains(l, "building configuration") ||
		strings.Contains(l, "/system") ||
		strings.Contains(l, "routeros") ||
		strings.Contains(s, "users.1.name") ||
		strings.Contains(s, "resolv.host.1.name") ||
		strings.Contains(s, "bridge.status=") ||
		strings.Contains(s, "httpd.https.status=")
}

func stripPager(s string) string {
	s = pagerMoreRe.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "\r", "")
	s = pagerQuitRe.ReplaceAllString(s, "")
	return s
}

var (
	pagerMoreRe = regexp.MustCompile(`(?i)--More--[^\r\n]*`)
	pagerQuitRe = regexp.MustCompile(`(?im)^[ \t]*or \(q\)uit[ \t]*$`)
)
