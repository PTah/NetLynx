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
	NativeVLAN   *int   // switchport trunk native vlan
	PortMode     string // access | trunk | …
	// Trunk allowed: All=true → «allowed vlan all»; иначе список (абсолютный / после add).
	TrunkAllowedAll bool
	TrunkAllowed    []int
	// TrunkAllowedMode — как в конфиге: all | set | add | remove | except | ""
	TrunkAllowedMode string
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
		if strings.HasPrefix(ll, "switchport trunk native vlan ") {
			rest := strings.TrimSpace(ll[len("switchport trunk native vlan"):])
			if n, err := strconv.Atoi(rest); err == nil && n > 0 && n <= 4094 {
				v := n
				s.NativeVLAN = &v
			}
		}
		if strings.HasPrefix(ll, "switchport trunk allowed vlan ") {
			rest := strings.TrimSpace(ll[len("switchport trunk allowed vlan"):])
			applyTrunkAllowedLive(&s, rest)
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
	// После полного блока: на trunk PVID/access без явного native → native.
	if strings.EqualFold(strings.TrimSpace(s.PortMode), "trunk") && s.NativeVLAN == nil && s.AccessVLAN != nil {
		v := *s.AccessVLAN
		s.NativeVLAN = &v
	}
	// Bare trunk без allowed-строки = all VLAN.
	if strings.EqualFold(strings.TrimSpace(s.PortMode), "trunk") && s.TrunkAllowedMode == "" {
		s.TrunkAllowedAll = true
		s.TrunkAllowedMode = TrunkAllowAll
	}
	return s
}

func applyTrunkAllowedLive(s *PortLiveSettings, rest string) {
	rest = strings.TrimSpace(rest)
	low := strings.ToLower(rest)
	switch {
	case low == "all":
		s.TrunkAllowedAll = true
		s.TrunkAllowedMode = TrunkAllowAll
		s.TrunkAllowed = nil
	case strings.HasPrefix(low, "add "):
		s.TrunkAllowedAll = false
		s.TrunkAllowedMode = TrunkAllowAdd
		s.TrunkAllowed = ParseVLANIDList(rest[len("add "):])
	case strings.HasPrefix(low, "remove "):
		s.TrunkAllowedAll = false
		s.TrunkAllowedMode = TrunkAllowRemove
		s.TrunkAllowed = ParseVLANIDList(rest[len("remove "):])
	case strings.HasPrefix(low, "except "):
		s.TrunkAllowedAll = false
		s.TrunkAllowedMode = TrunkAllowExcept
		s.TrunkAllowed = ParseVLANIDList(rest[len("except "):])
	default:
		s.TrunkAllowedAll = false
		s.TrunkAllowedMode = TrunkAllowSet
		s.TrunkAllowed = ParseVLANIDList(rest)
	}
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
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 25 * time.Second
	}
	enable := strings.TrimSpace(c.EnablePass)
	if enable == "" {
		enable = c.Password
	}
	client, err := DialSwitch(c)
	if err != nil {
		return PortLiveSettings{}, "", err
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
	// Педжер (--More--) waitQuiet не листает и тогда ждёт весь SSH-таймаут.
	// Баннер и show листаем пробелом и не дольше нескольких секунд.
	waitDump(in, &outBuf, 400*time.Millisecond, 4*time.Second)

	privCmd, confCmd, _ := confEnterCmds(v)
	send := func(s string, quiet time.Duration) error {
		if _, err := in.Write([]byte(s + "\r\n")); err != nil {
			return err
		}
		waitQuiet(&outBuf, quiet, timeout)
		return nil
	}
	show := func(s string, max time.Duration) error {
		if _, err := in.Write([]byte(s + "\r\n")); err != nil {
			return err
		}
		waitDump(in, &outBuf, 450*time.Millisecond, max)
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

	// Сначала короткий show одного порта — как в ручном SSH. На SNR current-mode
	// из (config) часто печатает весь конфиг и встаёт на --More-- до конца таймаута.
	shown := false
	for _, showCmd := range showInterfaceCmds(v, iface) {
		beforeShow := outBuf.Len()
		if err := show(showCmd, 8*time.Second); err != nil {
			return outBuf.String() + errBuf.String(), err
		}
		if interfaceSnippetOK(outBuf.String()[beforeShow:], iface) {
			shown = true
			break
		}
	}
	if v == VendorSNR && !shown {
		_ = send(confCmd, 500*time.Millisecond)
		_ = send("interface "+iface, 450*time.Millisecond)
		beforeShow := outBuf.Len()
		if err := show("show running-config current-mode", 8*time.Second); err != nil {
			return outBuf.String() + errBuf.String(), err
		}
		if !interfaceSnippetOK(outBuf.String()[beforeShow:], iface) {
			_ = send("exit", 300*time.Millisecond)
			_ = send("exit", 300*time.Millisecond)
		} else {
			_, _ = in.Write([]byte("end\r\n"))
		}
	}

	_, _ = in.Write([]byte("exit\r\n"))
	_ = in.Close()
	_ = sess.Wait()
	return outBuf.String() + errBuf.String(), nil
}

// showInterfaceCmds — команды чтения одного интерфейса, от более точной к запасной.
func showInterfaceCmds(v Vendor, iface string) []string {
	iface = strings.TrimSpace(iface)
	cmd := ShowInterfaceConfigCmd(v, iface)
	if v != VendorSNR {
		return []string{cmd}
	}
	cmds := []string{cmd}
	low := strings.ToLower(iface)
	if iface != "" && !strings.Contains(low, "ethernet") && !strings.Contains(low, "port-channel") && !strings.HasPrefix(low, "vlan") {
		alt := "show running-config interface ethernet " + iface
		if alt != cmd {
			cmds = append(cmds, alt)
		}
	}
	return cmds
}

// interfaceSnippetOK — в куске есть блок этого интерфейса, а не ошибка CLI.
func interfaceSnippetOK(chunk, iface string) bool {
	low := strings.ToLower(chunk)
	if strings.Contains(low, "invalid input") || strings.Contains(low, "% invalid") ||
		strings.Contains(low, "unknown command") || strings.Contains(low, "incomplete command") ||
		strings.Contains(low, "command not found") {
		return false
	}
	name := strings.ToLower(strings.TrimSpace(iface))
	if name != "" && strings.Contains(low, "interface "+name) {
		return true
	}
	return strings.Contains(low, "interface ")
}
