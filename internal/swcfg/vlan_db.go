package swcfg

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// ApplyVLANDatabaseChange создаёт VLAN, пишет имя или удаляет VLAN из vlan database.
// Порядок CLI зависит от вендора (см. vlanDBAttempts).
// Порядок попыток зависит от вендора (доки CLI); при Invalid input — следующий стиль.
func ApplyVLANDatabaseChange(c Creds, ch VLANDatabaseChange) error {
	if err := ch.Validate(); err != nil {
		return err
	}
	host := strings.TrimSpace(c.Host)
	user := strings.TrimSpace(c.User)
	if host == "" || user == "" {
		return fmt.Errorf("нет host или ssh user")
	}
	v := DetectVendor(string(c.Vendor), c.SysDescr, c.Name)
	if !SupportsPortCLI(v, c.SysDescr, c.Name) {
		return fmt.Errorf("запись vlan database не поддерживается (vendor=%s)", v)
	}
	if v == VendorAuto {
		v = DetectVendor("", c.SysDescr, c.Name)
	}
	if v == VendorMikrotik {
		return fmt.Errorf("RouterOS: vlan database пока не поддерживается")
	}

	attempts := vlanDBAttempts(v)
	var parts []string
	for _, a := range attempts {
		err := applyVLANDatabaseOnce(c, v, ch, a.style, a.enterConfig)
		if err == nil {
			return nil
		}
		if !cliLooksUnsupported(err) {
			return fmt.Errorf("%s: %w", a.label, err)
		}
		parts = append(parts, fmt.Sprintf("%s: %v", a.label, err))
	}
	if len(parts) == 0 {
		return fmt.Errorf("нет подходящего CLI для vlan database (vendor=%s)", v)
	}
	return fmt.Errorf("%s", strings.Join(parts, "; "))
}

type vlanDBAttempt struct {
	style       vlanCLIStyle
	enterConfig bool
	label       string
}

// vlanDBAttempts — порядок по документации вендоров (без живого железа в офисе).
//
//	Ubiquiti EdgeSwitch: privileged #vlan database → vlan name N "…"
//	ELTEX MES23xx: (config)# vlan database → vlan N name …
//	ELTEX MES14xx/24xx / SNR / Cisco / Aruba / Zyxel / HP / TP-Link / D-Link: vlan N / name
//	Huawei VRP: system-view → vlan N → name / undo name; undo vlan N
func vlanDBAttempts(v Vendor) []vlanDBAttempt {
	switch v {
	case VendorUbiquiti:
		return []vlanDBAttempt{
			{vlanStyleIEEE, false, "fastpath(#)"},
			{vlanStyleIEEE, true, "fastpath(config)"},
		}
	case VendorHuawei:
		return []vlanDBAttempt{
			{vlanStyleHuawei, true, "vrp"},
		}
	case VendorEltex:
		return []vlanDBAttempt{
			{vlanStyleEltex, true, "eltex-db"},
			{vlanStyleCisco, true, "cisco"},
			{vlanStyleIEEE, false, "fastpath(#)"},
		}
	case VendorSNR:
		return []vlanDBAttempt{
			{vlanStyleCisco, true, "cisco"},
			{vlanStyleIEEE, false, "fastpath(#)"},
			{vlanStyleIEEE, true, "fastpath(config)"},
			{vlanStyleEltex, true, "eltex-db"},
		}
	default:
		// Cisco, Aruba, Zyxel, HP ProCurve, TP-Link, D-Link, video switches.
		return []vlanDBAttempt{
			{vlanStyleCisco, true, "cisco"},
			{vlanStyleEltex, true, "eltex-db"},
			{vlanStyleIEEE, false, "fastpath(#)"},
			{vlanStyleIEEE, true, "fastpath(config)"},
		}
	}
}

func applyVLANDatabaseOnce(c Creds, v Vendor, ch VLANDatabaseChange, style vlanCLIStyle, enterConfig bool) error {
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
		return err
	}
	enable := strings.TrimSpace(c.EnablePass)
	if enable == "" {
		enable = c.Password
	}
	cfg := switchSSHConfig(c.User, c.Password, timeout, hk)
	addr := fmt.Sprintf("%s:%d", strings.TrimSpace(c.Host), port)
	client, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return fmt.Errorf("ssh %s: %w", c.Host, err)
	}
	defer client.Close()

	body := VLANDatabaseCLILines(style, ch)
	// Cisco IOS: vlan N / name — внутри configure.
	// EdgeSwitch/Fastpath: vlan database обычно из privileged EXEC (#).
	if enterConfig {
		body = append(body, confExitCmd(v), "write memory")
	} else {
		body = append(body, "write memory")
	}
	out, runErr := runVLANDBSession(client, timeout, v, enable, body, enterConfig)
	if ierr := interpretVLANDBCLI(out); ierr == nil {
		return nil
	} else if strings.TrimSpace(out) != "" {
		if runErr != nil && !isSSHSessionEOF(runErr) {
			return fmt.Errorf("%w; %s", ierr, runErr)
		}
		return ierr
	}
	if runErr != nil {
		if isSSHSessionEOF(runErr) {
			return fmt.Errorf("ssh: сессия закрылась до ответа CLI (EOF)")
		}
		return runErr
	}
	return fmt.Errorf("пустой вывод CLI")
}

func runVLANDBSession(client *ssh.Client, timeout time.Duration, v Vendor, enablePass string, body []string, enterConfig bool) (string, error) {
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
	waitQuiet(&outBuf, 500*time.Millisecond, timeout)

	privCmd, confCmd, writeCmd := confEnterCmds(v)
	send := func(s string, quiet time.Duration) error {
		if _, err := in.Write([]byte(s + "\r\n")); err != nil {
			return err
		}
		waitQuiet(&outBuf, quiet, timeout)
		return nil
	}
	if strings.TrimSpace(privCmd) != "" {
		before := outBuf.Len()
		if err := send(privCmd, 800*time.Millisecond); err != nil {
			return outBuf.String(), err
		}
		chunk := strings.ToLower(outBuf.String()[before:])
		needPass := strings.Contains(chunk, "password") || strings.Contains(chunk, "assword:")
		alreadyPriv := strings.Contains(chunk, "#") && !needPass
		if needPass && !alreadyPriv {
			if err := send(enablePass, 700*time.Millisecond); err != nil {
				return outBuf.String(), err
			}
		}
	}
	if enterConfig {
		if err := send(confCmd, 600*time.Millisecond); err != nil {
			return outBuf.String(), err
		}
	}
	for i, s := range body {
		if s == "write memory" {
			body[i] = writeCmd
		}
	}
	for _, s := range body {
		q := 450 * time.Millisecond
		low := strings.ToLower(s)
		if strings.HasPrefix(low, "vlan database") {
			q = 800 * time.Millisecond
		}
		if strings.HasPrefix(low, "write") || low == "save" || strings.HasPrefix(low, "save") {
			q = 2500 * time.Millisecond
		}
		if err := send(s, q); err != nil {
			return outBuf.String() + errBuf.String(), err
		}
		tail := strings.ToLower(outBuf.String())
		n := len(tail)
		if n > 240 {
			tail = tail[n-240:]
		}
		if (strings.HasPrefix(low, "write") || low == "save" || strings.HasPrefix(low, "save")) &&
			(strings.Contains(tail, "confirm") ||
				strings.Contains(tail, "(y/n)") ||
				strings.Contains(tail, "[y/n]")) {
			_ = send("y", 1500*time.Millisecond)
		}
	}
	_ = in.Close()
	_ = sess.Wait()
	return outBuf.String() + errBuf.String(), nil
}
