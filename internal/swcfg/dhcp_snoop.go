package swcfg

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// UbiquitiDHCPTrustCLI — команда на интерфейсе (также Eltex/SNR).
func UbiquitiDHCPTrustCLI(trusted bool) string {
	if trusted {
		return "ip dhcp snooping trust"
	}
	return "no ip dhcp snooping trust"
}

// CountDHCPSnoopingTrustLines считает порты с trust в running-config.
func CountDHCPSnoopingTrustLines(cfg string) int {
	n := 0
	for _, raw := range strings.Split(cfg, "\n") {
		ll := strings.ToLower(strings.TrimSpace(raw))
		if ll == "ip dhcp snooping trust" {
			n++
		}
	}
	return n
}

// ApplyDHCPSnoopingTrust ставит/снимает trust; управляет глобальным ip dhcp snooping.
func ApplyDHCPSnoopingTrust(c Creds, iface string, trusted bool) error {
	iface = strings.TrimSpace(iface)
	if iface == "" {
		return fmt.Errorf("пустое имя интерфейса")
	}
	host := strings.TrimSpace(c.Host)
	user := strings.TrimSpace(c.User)
	if host == "" || user == "" {
		return fmt.Errorf("нет host или ssh user")
	}
	v := DetectVendor(string(c.Vendor), c.SysDescr, c.Name)
	if !SupportsPortCLI(v, c.SysDescr, c.Name) {
		return fmt.Errorf("DHCP Snooping trust не поддерживается (vendor=%s)", v)
	}
	if v == VendorAuto {
		v = DetectVendor("", c.SysDescr, c.Name)
	}
	port := c.Port
	if port <= 0 {
		port = 22
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	hk, err := HostKeyCallback(c.KnownHosts)
	if err != nil {
		return err
	}
	enable := strings.TrimSpace(c.EnablePass)
	if enable == "" {
		enable = c.Password
	}
	cfg := switchSSHConfig(user, c.Password, timeout, hk)
	addr := fmt.Sprintf("%s:%d", host, port)
	client, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return fmt.Errorf("ssh %s: %w", host, err)
	}
	defer client.Close()

	out, runErr := runDHCPSnoopingTrust(client, timeout, v, enable, iface, trusted)
	if ierr := interpretDHCPSnoopCLI(out, trusted); ierr == nil {
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

func runDHCPSnoopingTrust(client *ssh.Client, timeout time.Duration, v Vendor, enablePass, iface string, trusted bool) (string, error) {
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
	_ = send("terminal length 0", 400*time.Millisecond)
	if err := send(confCmd, 600*time.Millisecond); err != nil {
		return outBuf.String(), err
	}

	if trusted {
		if err := send("ip dhcp snooping", 500*time.Millisecond); err != nil {
			return outBuf.String(), err
		}
		if err := send("interface "+iface, 450*time.Millisecond); err != nil {
			return outBuf.String(), err
		}
		if err := send(DHCPTrustCLI(true), 450*time.Millisecond); err != nil {
			return outBuf.String(), err
		}
		_ = send("exit", 350*time.Millisecond)
		_ = send("exit", 350*time.Millisecond)
		if err := send(writeCmd, 2500*time.Millisecond); err != nil {
			return outBuf.String() + errBuf.String(), err
		}
		maybeConfirmWrite(&outBuf, send)
	} else {
		if err := send("interface "+iface, 450*time.Millisecond); err != nil {
			return outBuf.String(), err
		}
		if err := send(DHCPTrustCLI(false), 450*time.Millisecond); err != nil {
			return outBuf.String(), err
		}
		_ = send("exit", 350*time.Millisecond)
		_ = send("exit", 350*time.Millisecond)

		beforeShow := outBuf.Len()
		includeCmd := "show running-config | include ip dhcp snooping trust"
		if err := send(includeCmd, 0); err != nil {
			return outBuf.String() + errBuf.String(), err
		}
		waitDump(in, &outBuf, 700*time.Millisecond, 15*time.Second)
		cfgChunk := outBuf.String()[beforeShow:]
		remain := CountDHCPSnoopingTrustLines(cfgChunk)
		if remain == 0 {
			if err := send(confCmd, 600*time.Millisecond); err != nil {
				return outBuf.String(), err
			}
			if err := send("no ip dhcp snooping", 500*time.Millisecond); err != nil {
				return outBuf.String(), err
			}
			_ = send("exit", 350*time.Millisecond)
		}
		if err := send(writeCmd, 2500*time.Millisecond); err != nil {
			return outBuf.String() + errBuf.String(), err
		}
		maybeConfirmWrite(&outBuf, send)
	}

	_ = in.Close()
	_ = sess.Wait()
	return outBuf.String() + errBuf.String(), nil
}

func maybeConfirmWrite(outBuf *bytes.Buffer, send func(string, time.Duration) error) {
	tail := strings.ToLower(outBuf.String())
	n := len(tail)
	if n > 240 {
		tail = tail[n-240:]
	}
	if strings.Contains(tail, "confirm") || strings.Contains(tail, "(y/n)") || strings.Contains(tail, "[y/n]") {
		_ = send("y", 1500*time.Millisecond)
	}
}

func interpretDHCPSnoopCLI(out string, trusted bool) error {
	low := strings.ToLower(out)
	idx := strings.Index(low, "(config")
	if idx < 0 {
		idx = strings.Index(low, "config)#")
	}
	check := low
	if idx >= 0 {
		check = low[idx:]
	}
	for _, bad := range []string{
		"invalid input detected",
		"incomplete command",
		"command not found",
		"unknown command",
		"% invalid",
		"permission denied",
		"authorization failed",
	} {
		if strings.Contains(check, bad) {
			return fmt.Errorf("cli: %s — %s", bad, compactCLIErr(out))
		}
	}
	ok := strings.Contains(low, "dhcp snooping") ||
		strings.Contains(low, "(config") ||
		strings.Contains(low, "interface ")
	if !ok {
		return fmt.Errorf("cli: нет признаков dhcp snooping — %s", compactCLIErr(out))
	}
	_ = trusted
	return nil
}
