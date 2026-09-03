package swcfg

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// PortLiveSettings — снимок настроек порта с устройства.
type PortLiveSettings struct {
	AdminUp      bool
	Isolate      bool
	PoEMode      string // off | 24v | poe+
	DHCPTrusted  bool
	FlowControl  bool
	STPEnabled   bool
	EdgePort     string // auto | enable | disable
	PortPriority int
	PathCost     int // 0 = auto
	AccessVLAN   *int
	PortMode     string // access | trunk | …
}

// ParseInterfaceFromRunningConfig — настройки одного interface из полного running-config.
// ok=false, если блок interface не найден.
func ParseInterfaceFromRunningConfig(full, iface string) (PortLiveSettings, bool) {
	iface = strings.TrimSpace(iface)
	full = strings.TrimSpace(full)
	if iface == "" || full == "" {
		return PortLiveSettings{}, false
	}
	needle := "interface " + strings.ToLower(iface)
	restFull := full
	restLow := strings.ToLower(full)
	for {
		idx := strings.Index(restLow, needle)
		if idx < 0 {
			return PortLiveSettings{}, false
		}
		after := idx + len(needle)
		if after >= len(restLow) || restLow[after] == ' ' || restLow[after] == '\t' || restLow[after] == '\r' || restLow[after] == '\n' {
			snip := restFull[idx:]
			return ParseInterfaceConfigSnippet(snip, iface), true
		}
		restLow = restLow[after:]
		restFull = restFull[after:]
	}
}

// ParseInterfaceConfigSnippet разбирает вывод show running-config interface …
// wantIface — опционально: искать именно этот интерфейс (иначе первый interface в выводе).
func ParseInterfaceConfigSnippet(out string, wantIface ...string) PortLiveSettings {
	s := PortLiveSettings{
		AdminUp:      true,
		Isolate:      false,
		PoEMode:      "poe+",
		DHCPTrusted:  false,
		FlowControl:  false,
		STPEnabled:   true, // по умолчанию STP на порту обычно включён
		EdgePort:     "auto",
		PortPriority: 128,
		PathCost:     0,
	}
	want := ""
	if len(wantIface) > 0 {
		want = strings.ToLower(strings.TrimSpace(wantIface[0]))
	}
	low := strings.ToLower(out)
	body := out
	if want != "" {
		// Ищем «interface <name>» (с пробелами/кавычками).
		needle := "interface " + want
		if idx := strings.Index(low, needle); idx >= 0 {
			body = out[idx:]
		} else if idx := strings.Index(low, "interface "); idx >= 0 {
			body = out[idx:]
		}
	} else if idx := strings.Index(low, "interface "); idx >= 0 {
		body = out[idx:]
	}
	sawSTPDisable := false
	sawSTPEnable := false
	inBlock := false
	for _, raw := range strings.Split(body, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		// Убрать хвостовые комментарии Fastpath (! …) / Cisco (!…).
		if i := strings.Index(line, "!"); i >= 0 {
			line = strings.TrimSpace(line[:i])
			if line == "" {
				continue
			}
		}
		ll := strings.ToLower(line)
		if strings.HasPrefix(ll, "interface ") {
			if inBlock {
				break
			}
			inBlock = true
			continue
		}
		if !inBlock {
			continue
		}
		if ll == "exit" || ll == "end" || ll == "!" {
			break
		}
		if ll == "shutdown" || strings.HasPrefix(ll, "shutdown ") {
			s.AdminUp = false
		}
		if ll == "no shutdown" || strings.HasPrefix(ll, "no shutdown ") {
			s.AdminUp = true
		}
		// Некоторые CLI: «admin state disable» / «adminstate disable».
		if (strings.Contains(ll, "admin state") || strings.Contains(ll, "adminstate") || strings.Contains(ll, "admin-state")) &&
			strings.Contains(ll, "disable") {
			s.AdminUp = false
		}
		if (strings.Contains(ll, "admin state") || strings.Contains(ll, "adminstate") || strings.Contains(ll, "admin-state")) &&
			strings.Contains(ll, "enable") && !strings.Contains(ll, "disable") {
			s.AdminUp = true
		}
		if strings.HasPrefix(ll, "switchport protected") || ll == "switchport protected-port" {
			s.Isolate = true
		}
		if ll == "ip dhcp snooping trust" {
			s.DHCPTrusted = true
		}
		if ll == "flowcontrol" || ll == "flow control" || ll == "flowcontrol mode on" {
			s.FlowControl = true
		}
		if ll == "no flowcontrol" || ll == "no flow control" || ll == "flowcontrol mode off" {
			s.FlowControl = false
		}
		if strings.HasPrefix(ll, "poe opmode") {
			rest := strings.TrimSpace(ll[len("poe opmode"):])
			switch {
			case strings.Contains(rest, "shutdown"):
				s.PoEMode = "off"
			case strings.Contains(rest, "passive24v") || strings.Contains(rest, "24v"):
				s.PoEMode = "24v"
			case strings.Contains(rest, "auto"):
				s.PoEMode = "poe+"
			}
		}
		if ll == "power inline never" || ll == "no power inline enable" || ll == "no power inline" {
			s.PoEMode = "off"
		}
		if ll == "power inline auto" || ll == "power inline enable" || strings.HasPrefix(ll, "power inline auto") {
			s.PoEMode = "poe+"
		}
		// STP enable/disable
		if ll == "no spanning-tree port mode" || ll == "spanning-tree disable" || ll == "no spanning-tree" {
			sawSTPDisable = true
		}
		if ll == "spanning-tree port mode" || ll == "no spanning-tree disable" || ll == "spanning-tree" {
			sawSTPEnable = true
		}
		if strings.HasPrefix(ll, "spanning-tree edgeport") {
			if strings.Contains(ll, "auto") {
				s.EdgePort = "auto"
			} else {
				s.EdgePort = "enable"
			}
		}
		if ll == "no spanning-tree edgeport" {
			s.EdgePort = "disable"
		}
		if strings.HasPrefix(ll, "spanning-tree portfast") {
			if strings.Contains(ll, "auto") {
				s.EdgePort = "auto"
			} else {
				s.EdgePort = "enable"
			}
		}
		if ll == "no spanning-tree portfast" {
			s.EdgePort = "disable"
		}
		if strings.HasPrefix(ll, "spanning-tree port-priority") {
			var p int
			fmt.Sscanf(ll, "spanning-tree port-priority %d", &p)
			if p >= 0 && p <= 240 {
				s.PortPriority = p
			}
		}
		if strings.HasPrefix(ll, "spanning-tree cost") {
			var c int
			fmt.Sscanf(ll, "spanning-tree cost %d", &c)
			if c > 0 {
				s.PathCost = c
			}
		}
		if ll == "no spanning-tree cost" {
			s.PathCost = 0
		}
		if strings.HasPrefix(ll, "switchport mode ") {
			s.PortMode = strings.TrimSpace(ll[len("switchport mode"):])
		}
		if strings.HasPrefix(ll, "switchport access vlan ") {
			rest := strings.TrimSpace(ll[len("switchport access vlan"):])
			if n, err := strconv.Atoi(rest); err == nil && n > 0 && n <= 4094 {
				v := n
				s.AccessVLAN = &v
			}
		}
		if strings.HasPrefix(ll, "vlan pvid ") {
			rest := strings.TrimSpace(ll[len("vlan pvid"):])
			if n, err := strconv.Atoi(rest); err == nil && n > 0 && n <= 4094 && s.AccessVLAN == nil {
				v := n
				s.AccessVLAN = &v
			}
		}
	}
	if sawSTPDisable {
		s.STPEnabled = false
	} else if sawSTPEnable {
		s.STPEnabled = true
	}
	return s
}

// FetchPortLiveSettings читает running-config одного интерфейса.
func FetchPortLiveSettings(c Creds, iface string) (PortLiveSettings, string, error) {
	iface = strings.TrimSpace(iface)
	if iface == "" {
		return PortLiveSettings{}, "", fmt.Errorf("пустое имя интерфейса")
	}
	host := strings.TrimSpace(c.Host)
	user := strings.TrimSpace(c.User)
	if host == "" || user == "" {
		return PortLiveSettings{}, "", fmt.Errorf("нет host или ssh user")
	}
	v := DetectVendor(string(c.Vendor), c.SysDescr, c.Name)
	if !SupportsPortCLI(v, c.SysDescr, c.Name) {
		return PortLiveSettings{}, "", fmt.Errorf("чтение live-настроек не поддерживается (vendor=%s)", v)
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
		timeout = 25 * time.Second
	}
	hk, err := HostKeyCallback(c.KnownHosts)
	if err != nil {
		return PortLiveSettings{}, "", err
	}
	enable := strings.TrimSpace(c.EnablePass)
	if enable == "" {
		enable = c.Password
	}
	cfg := switchSSHConfig(user, c.Password, timeout, hk)
	addr := fmt.Sprintf("%s:%d", host, port)
	client, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return PortLiveSettings{}, "", fmt.Errorf("ssh %s: %w", host, err)
	}
	defer client.Close()

	out, err := runShowInterfaceConfig(client, timeout, v, enable, iface)
	if err != nil && strings.TrimSpace(out) == "" {
		return PortLiveSettings{}, out, err
	}
	if !strings.Contains(strings.ToLower(out), "interface") {
		snip := compactCLIErr(out)
		if err != nil {
			return PortLiveSettings{}, out, fmt.Errorf("нет блока interface: %v; %s", err, snip)
		}
		return PortLiveSettings{}, out, fmt.Errorf("нет блока interface: %s", snip)
	}
	return ParseInterfaceConfigSnippet(out, iface), out, nil
}

func runShowInterfaceConfig(client *ssh.Client, timeout time.Duration, v Vendor, enablePass, iface string) (string, error) {
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
	waitQuiet(&outBuf, 400*time.Millisecond, timeout)

	privCmd, confCmd, _ := confEnterCmds(v)
	send := func(s string, quiet time.Duration) error {
		if _, err := in.Write([]byte(s + "\r\n")); err != nil {
			return err
		}
		waitQuiet(&outBuf, quiet, timeout)
		return nil
	}

	before := outBuf.Len()
	if err := send(privCmd, 700*time.Millisecond); err != nil {
		return outBuf.String(), err
	}
	chunk := strings.ToLower(outBuf.String()[before:])
	needPass := strings.Contains(chunk, "password") || strings.Contains(chunk, "assword:")
	alreadyPriv := strings.Contains(chunk, "#") && !needPass
	if needPass && !alreadyPriv {
		if err := send(enablePass, 600*time.Millisecond); err != nil {
			return outBuf.String(), err
		}
	}
	_ = send("terminal length 0", 400*time.Millisecond)
	if v == VendorEltex {
		_ = send("terminal datadump", 400*time.Millisecond)
	}

	showCmd := ShowInterfaceConfigCmd(v, iface)
	if v == VendorSNR {
		// fallback path: config → interface → show running-config current-mode
		_ = send(confCmd, 500*time.Millisecond)
		_ = send("interface "+iface, 450*time.Millisecond)
		beforeShow := outBuf.Len()
		if err := send("show running-config current-mode", 0); err != nil {
			return outBuf.String() + errBuf.String(), err
		}
		waitDump(in, &outBuf, 600*time.Millisecond, 12*time.Second)
		chunk := outBuf.String()[beforeShow:]
		if strings.Contains(strings.ToLower(chunk), "interface") || len(strings.TrimSpace(chunk)) > 40 {
			_, _ = in.Write([]byte("exit\r\nexit\r\n"))
			_ = in.Close()
			_ = sess.Wait()
			return outBuf.String() + errBuf.String(), nil
		}
		_ = send("exit", 300*time.Millisecond)
		_ = send("exit", 300*time.Millisecond)
	}

	if err := send(showCmd, 0); err != nil {
		return outBuf.String() + errBuf.String(), err
	}
	dumpMax := timeout
	if dumpMax > 20*time.Second {
		dumpMax = 20 * time.Second
	}
	if dumpMax < 8*time.Second {
		dumpMax = 8 * time.Second
	}
	waitDump(in, &outBuf, 600*time.Millisecond, dumpMax)
	_, _ = in.Write([]byte("exit\r\n"))
	_ = in.Close()
	_ = sess.Wait()
	return outBuf.String() + errBuf.String(), nil
}
