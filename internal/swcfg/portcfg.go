package swcfg

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// PortChange — изменение порта через CLI (configure → interface → …).
type PortChange struct {
	Interface string // ifName, напр. "0/1" или "GigabitEthernet 1/0/1"
	// Description: nil = не трогать; указатель на "" = снять description.
	Description *string
	// AdminUp: nil = не трогать; true = no shutdown; false = shutdown.
	AdminUp *bool
	// PoEMode: nil = не трогать; "off" | "24v" | "poe+"
	PoEMode *string
	// Isolate: nil = не трогать
	Isolate *bool
	// FlowControl: nil = не трогать
	FlowControl *bool
	// STP: nil = не трогать; частичные поля внутри
	STP *STPChange
	// VLAN: nil = не трогать
	VLAN *PortVLANChange
	// vlanStyle: Auto = cisco, затем 802.1Q при Invalid input.
	vlanStyle vlanCLIStyle
	Write     bool
}

func (ch PortChange) empty() bool {
	return ch.Description == nil && ch.AdminUp == nil && ch.PoEMode == nil &&
		ch.Isolate == nil && ch.FlowControl == nil && ch.STP == nil && ch.VLAN == nil
}

// ApplyPortChange заходит в privileged + configure и применяет изменения порта.
func ApplyPortChange(c Creds, ch PortChange) error {
	iface := strings.TrimSpace(ch.Interface)
	if iface == "" {
		return fmt.Errorf("пустое имя интерфейса")
	}
	if ch.empty() {
		return fmt.Errorf("нечего менять")
	}
	host := strings.TrimSpace(c.Host)
	user := strings.TrimSpace(c.User)
	if host == "" || user == "" {
		return fmt.Errorf("нет host или ssh user")
	}
	v := DetectVendor(string(c.Vendor), c.SysDescr, c.Name)
	if !SupportsPortCLI(v, c.SysDescr, c.Name) {
		return fmt.Errorf("запись настроек порта не поддерживается (vendor=%s)", v)
	}
	if v == VendorAuto {
		v = DetectVendor("", c.SysDescr, c.Name)
	}
	if v == VendorMikrotik {
		if ch.VLAN != nil {
			return fmt.Errorf("RouterOS: VLAN на порту пока не поддерживается")
		}
		return applyMikrotikPortChange(c, ch)
	}
	if ch.VLAN != nil {
		if err := ch.VLAN.Validate(); err != nil {
			return err
		}
		if ch.vlanStyle == vlanStyleAuto {
			cisco := ch
			cisco.vlanStyle = vlanStyleCisco
			err := ApplyPortChange(c, cisco)
			if err == nil {
				return nil
			}
			if !cliLooksUnsupported(err) {
				return err
			}
			ieee := ch
			ieee.vlanStyle = vlanStyleIEEE
			err2 := ApplyPortChange(c, ieee)
			if err2 == nil {
				return nil
			}
			return fmt.Errorf("cisco-style: %v; 802.1Q: %v", err, err2)
		}
	}
	if ch.PoEMode != nil {
		if _, err := PoEModeCLI(v, *ch.PoEMode); err != nil {
			return err
		}
	}
	if ch.STP != nil {
		if _, err := STPCLILines(v, *ch.STP); err != nil {
			return err
		}
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

	out, runErr := runPortConfigure(client, timeout, v, enable, iface, ch)
	if ierr := interpretPortCLI(out); ierr == nil {
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

func isSSHSessionEOF(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) {
		return true
	}
	s := strings.ToLower(err.Error())
	return s == "eof" || strings.Contains(s, "eof")
}

func portConfigBody(v Vendor, iface string, ch PortChange) ([]string, error) {
	var steps []string
	if ch.Isolate != nil && (v == VendorAruba || v == VendorZyxel || v == VendorHP ||
		v == VendorTPLink || v == VendorDLink ||
		v == VendorDahua || v == VendorHikvision || v == VendorHiWatch || v == VendorTrassir) {
		return nil, fmt.Errorf("%s: изоляция порта пока не поддерживается", v)
	}
	// SNR isolate — глобальные команды до/вместо interface
	if ch.Isolate != nil && v == VendorSNR {
		steps = append(steps, SNRIsolateSteps(iface, *ch.Isolate)...)
	}

	needIF := ch.Description != nil || ch.AdminUp != nil || ch.PoEMode != nil ||
		ch.FlowControl != nil || ch.STP != nil || ch.VLAN != nil ||
		(ch.Isolate != nil && v != VendorSNR)

	if needIF {
		steps = append(steps, "interface "+iface)
		if ch.Description != nil {
			d := strings.TrimSpace(*ch.Description)
			d = strings.ReplaceAll(d, "\r", " ")
			d = strings.ReplaceAll(d, "\n", " ")
			d = strings.ReplaceAll(d, `"`, "")
			for strings.Contains(d, "  ") {
				d = strings.ReplaceAll(d, "  ", " ")
			}
			if d == "" {
				steps = append(steps, ClearDescriptionCLI(v))
			} else {
				steps = append(steps, "description "+quoteCLIDescription(d))
			}
		}
		if ch.AdminUp != nil {
			steps = append(steps, AdminUpCLI(v, *ch.AdminUp))
		}
		if ch.PoEMode != nil {
			cmd, err := PoEModeCLI(v, *ch.PoEMode)
			if err != nil {
				return nil, err
			}
			steps = append(steps, cmd)
		}
		if ch.Isolate != nil && v != VendorSNR {
			iso := IsolateCLI(v, *ch.Isolate)
			if len(iso) == 0 {
				return nil, fmt.Errorf("%s: изоляция порта пока не поддерживается", v)
			}
			steps = append(steps, iso...)
		}
		if ch.FlowControl != nil {
			steps = append(steps, FlowControlCLI(v, *ch.FlowControl))
		}
		if ch.STP != nil {
			stp, err := STPCLILines(v, *ch.STP)
			if err != nil {
				return nil, err
			}
			steps = append(steps, stp...)
		}
		if ch.VLAN != nil {
			st := ch.vlanStyle
			if st == vlanStyleAuto {
				st = vlanStyleCisco
			}
			vl := VLANCLILines(st, *ch.VLAN)
			if len(vl) == 0 {
				return nil, fmt.Errorf("vlan: операция %q не поддерживается этим CLI-стилем", ch.VLAN.Op)
			}
			steps = append(steps, vl...)
		}
		steps = append(steps, ifaceExitCmd(v))
	}
	steps = append(steps, confExitCmd(v))
	if ch.Write {
		steps = append(steps, "write memory")
	}
	return steps, nil
}

// quoteCLIDescription — EdgeSwitch/Cisco: пробелы и дефисы требуют кавычек в running-config.
func quoteCLIDescription(d string) string {
	d = strings.TrimSpace(d)
	if d == "" {
		return ""
	}
	if !strings.ContainsAny(d, " \t-") {
		return d
	}
	if strings.Contains(d, `'`) {
		return `"` + strings.ReplaceAll(d, `"`, "") + `"`
	}
	return `'` + d + `'`
}

func runPortConfigure(client *ssh.Client, timeout time.Duration, v Vendor, enablePass, iface string, ch PortChange) (string, error) {
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

	if err := send(confCmd, 600*time.Millisecond); err != nil {
		return outBuf.String(), err
	}

	body, err := portConfigBody(v, iface, ch)
	if err != nil {
		return outBuf.String(), err
	}
	for i, s := range body {
		if s == "write memory" {
			body[i] = writeCmd
		}
	}
	for _, s := range body {
		q := 450 * time.Millisecond
		low := strings.ToLower(s)
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
				strings.Contains(tail, "[y/n]") ||
				strings.Contains(tail, "[y/n]:")) {
			_ = send("y", 1500*time.Millisecond)
		}
	}

	_ = in.Close()
	_ = sess.Wait()
	return outBuf.String() + errBuf.String(), nil
}

func interpretPortCLI(out string) error {
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
	ok := strings.Contains(low, "(config") ||
		strings.Contains(low, "config)#") ||
		strings.Contains(low, "system-view") ||
		strings.Contains(low, "[*") ||
		strings.Contains(low, "interface ") ||
		strings.Contains(low, "\ndescription ") ||
		strings.Contains(low, " no description") ||
		strings.Contains(low, "undo description") ||
		strings.Contains(low, "no shutdown") ||
		strings.Contains(low, "undo shutdown") ||
		strings.Contains(low, "\nshutdown") ||
		strings.Contains(low, "poe opmode") ||
		strings.Contains(low, "power inline") ||
		strings.Contains(low, "power-over-ethernet") ||
		strings.Contains(low, "poe mode") ||
		strings.Contains(low, "poe enable") ||
		strings.Contains(low, "switchport protected") ||
		strings.Contains(low, "port-isolate") ||
		strings.Contains(low, "isolate-port") ||
		strings.Contains(low, "flowcontrol") ||
		strings.Contains(low, "flow control") ||
		strings.Contains(low, "flow-control") ||
		strings.Contains(low, "spanning-tree")
	if !ok {
		return fmt.Errorf("cli: нет признаков configure/interface — %s", compactCLIErr(out))
	}
	return nil
}
