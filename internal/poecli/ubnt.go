package poecli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/snmp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

var portLineRe = regexp.MustCompile(`^\s*([0-9]+/[0-9]+(?:/[0-9]+)?)\s+(.+)$`)
var poeStatusAllLineRe = regexp.MustCompile(`^\s*([0-9]+/[0-9]+(?:/[0-9]+)?)\s+(.+?)\s+(\S+)\s+([0-9]*[.,]?[0-9]+)\s+([0-9]*[.,]?[0-9]+)\s+([0-9]*[.,]?[0-9]+)\s+([0-9]*[.,]?[0-9]+)\s+([0-9]+)\s*$`)
var snrPowerInlineLineRe = regexp.MustCompile(`^\s*(ethernet[0-9]+/[0-9]+/[0-9]+)\s+\S+\s+(\S+)\s+([0-9]*[.,]?[0-9]+)\s+.*$`)

type sshVendorProfile struct {
	name            string
	privilegeCmd    string
	needsEnablePass bool
	poeCommands     []string
}

func selectSSHVendorProfile(sysDescr string) sshVendorProfile {
	s := strings.ToLower(sysDescr)
	switch {
	case strings.Contains(s, "edgeswitch"), strings.Contains(s, "ubnt"), strings.Contains(s, "ubiquiti"):
		return sshVendorProfile{
			name:            "ubiquiti",
			privilegeCmd:    "en",
			needsEnablePass: true,
			poeCommands:     []string{"show poe status all", "show poe status"},
		}
	case strings.Contains(s, "mikrotik"), strings.Contains(s, "routeros"),
		strings.Contains(s, "ccr"), strings.Contains(s, "crs"), strings.Contains(s, "routerboard"):
		return sshVendorProfile{
			name:            "mikrotik",
			privilegeCmd:    "",
			needsEnablePass: false,
			poeCommands:     []string{"/interface ethernet poe print detail without-paging", "/interface ethernet poe print"},
		}
	case strings.Contains(s, "huawei"), strings.Contains(s, "vrp"):
		return sshVendorProfile{
			name:            "huawei",
			privilegeCmd:    "",
			needsEnablePass: false,
			poeCommands:     []string{"display poe power", "display poe interface"},
		}
	case strings.Contains(s, "aruba"):
		return sshVendorProfile{
			name:            "aruba",
			privilegeCmd:    "enable",
			needsEnablePass: true,
			poeCommands:     []string{"show power-over-ethernet", "show power-over-ethernet all", "show poe"},
		}
	case strings.Contains(s, "procurve"), strings.Contains(s, "hewlett-packard"):
		return sshVendorProfile{
			name:            "hp",
			privilegeCmd:    "enable",
			needsEnablePass: true,
			poeCommands:     []string{"show power-over-ethernet", "show power-over-ethernet brief", "show poe"},
		}
	case strings.Contains(s, "zyxel"):
		return sshVendorProfile{
			name:            "zyxel",
			privilegeCmd:    "enable",
			needsEnablePass: true,
			poeCommands:     []string{"show poe", "show power inline"},
		}
	case strings.Contains(s, "tp-link"), strings.Contains(s, "tplink"), strings.Contains(s, "jetstream"):
		return sshVendorProfile{
			name:            "tplink",
			privilegeCmd:    "enable",
			needsEnablePass: true,
			poeCommands:     []string{"show power inline", "show poe"},
		}
	case strings.Contains(s, "d-link"), strings.Contains(s, "dlink"), strings.Contains(s, "dgs-"), strings.Contains(s, "des-"):
		return sshVendorProfile{
			name:            "dlink",
			privilegeCmd:    "enable",
			needsEnablePass: true,
			poeCommands:     []string{"show power inline", "show poe"},
		}
	case strings.Contains(s, "dahua"), strings.Contains(s, "dh-pfs"), strings.Contains(s, "pfs42"):
		return sshVendorProfile{
			name:            "dahua",
			privilegeCmd:    "enable",
			needsEnablePass: true,
			poeCommands:     []string{"show poe", "show power inline"},
		}
	case strings.Contains(s, "hikvision"), strings.Contains(s, "ds-3e"), strings.Contains(s, "hiwatch"):
		return sshVendorProfile{
			name:            "hikvision",
			privilegeCmd:    "enable",
			needsEnablePass: true,
			poeCommands:     []string{"show poe", "show power inline"},
		}
	case strings.Contains(s, "trassir"):
		return sshVendorProfile{
			name:            "trassir",
			privilegeCmd:    "enable",
			needsEnablePass: true,
			poeCommands:     []string{"show poe", "show power inline"},
		}
	case strings.Contains(s, "cisco"), strings.Contains(s, "nx-os"), strings.Contains(s, "catalyst"):
		return sshVendorProfile{
			name:            "cisco",
			privilegeCmd:    "enable",
			needsEnablePass: true,
			poeCommands:     []string{"show power inline"},
		}
	case strings.Contains(s, "snr"), strings.Contains(s, "nag llc"):
		return sshVendorProfile{
			name:            "snr",
			privilegeCmd:    "",
			needsEnablePass: false,
			poeCommands:     []string{"show power inline"},
		}
	default:
		return sshVendorProfile{
			name:            "generic",
			privilegeCmd:    "",
			needsEnablePass: false,
			poeCommands:     []string{"show poe status all", "show poe status", "show power inline"},
		}
	}
}

// ReadPoEActiveByIfIndex выполняет вендор-специфичный SSH CLI для PoE и возвращает только порты с активной выдачей питания.
// Карта содержит ifIndex->true. Значения false намеренно не возвращаются, чтобы не затирать состояние при неполном выводе CLI.
// knownHostsPath — файл known_hosts (SSH_POE_KNOWN_HOSTS); без него dial запрещён (нет InsecureIgnoreHostKey).
func ReadPoEActiveByIfIndex(host string, port int, user, pass, enablePass, sysDescr string, ifRows map[int]snmp.IfRow, timeout time.Duration, knownHostsPath string) (map[int]bool, error) {
	if strings.TrimSpace(host) == "" || strings.TrimSpace(user) == "" {
		return nil, fmt.Errorf("ssh poe: empty host/user")
	}
	if port <= 0 {
		port = 22
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	hkcb, err := hostKeyCallback(knownHostsPath)
	if err != nil {
		return nil, err
	}

	cfg := &ssh.ClientConfig{
		User:            strings.TrimSpace(user),
		Auth:            []ssh.AuthMethod{ssh.Password(pass)},
		HostKeyCallback: hkcb,
		Timeout:         timeout,
	}
	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", host, port), cfg)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	profile := selectSSHVendorProfile(sysDescr)
	out, err := runShowPoeStatus(client, pass, enablePass, profile)
	if err != nil {
		return nil, err
	}
	return parseUbiquitiShowPoeStatusToIfIndex(out, ifRows), nil
}

func hostKeyCallback(knownHostsPath string) (ssh.HostKeyCallback, error) {
	path := strings.TrimSpace(knownHostsPath)
	if path == "" {
		return nil, fmt.Errorf("ssh poe: задайте SSH_POE_KNOWN_HOSTS (путь к known_hosts с ключами свитчей)")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	f, err := os.Open(abs)
	if err != nil {
		return nil, fmt.Errorf("ssh poe: known_hosts %s: %w", abs, err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("ssh poe: known_hosts stat: %w", err)
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("ssh poe: known_hosts не обычный файл")
	}
	// POSIX: запрет world-writable (и group-writable для жёсткости).
	if st.Mode().Perm()&0o022 != 0 {
		return nil, fmt.Errorf("ssh poe: known_hosts %s слишком открыт (права %#o, уберите write для group/other)", abs, st.Mode().Perm())
	}
	cb, err := knownhosts.New(abs)
	if err != nil {
		return nil, fmt.Errorf("ssh poe: known_hosts: %w", err)
	}
	return cb, nil
}

func runShowPoeStatus(client *ssh.Client, loginPass, enablePass string, profile sshVendorProfile) (string, error) {
	if strings.TrimSpace(enablePass) == "" {
		enablePass = loginPass
	}
	// Для профилей с privileged mode используем один интерактивный канал и не открываем дополнительные сессии:
	// часть устройств закрывает SSH после "exit", что даёт "unexpected packet ... channel open" на повторном NewSession.
	if profile.privilegeCmd != "" {
		out, err := runShowPoeStatusWithProfile(client, enablePass, profile)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(out) == "" {
			return "", fmt.Errorf("ssh poe: empty CLI output")
		}
		return out, nil
	}

	commands := profile.poeCommands
	if len(commands) == 0 {
		commands = []string{"show poe status all", "show poe status", "show power inline"}
	}
	var lastOut string
	for _, cmd := range commands {
		sess, err := client.NewSession()
		if err != nil {
			return "", err
		}
		b, runErr := sess.CombinedOutput(cmd)
		_ = sess.Close()
		s := string(b)
		lastOut = s
		if runErr != nil {
			continue
		}
		if looksLikeUnsupportedCommand(s) {
			continue
		}
		return s, nil
	}
	if strings.TrimSpace(lastOut) == "" {
		return "", fmt.Errorf("ssh poe: empty CLI output")
	}
	return "", fmt.Errorf("ssh poe: no supported command output")
}

func runShowPoeStatusWithProfile(client *ssh.Client, enablePass string, profile sshVendorProfile) (string, error) {
	sess, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer sess.Close()

	in, err := sess.StdinPipe()
	if err != nil {
		return "", err
	}

	var outBuf bytes.Buffer
	var errBuf bytes.Buffer
	sess.Stdout = &outBuf
	sess.Stderr = &errBuf

	if err := sess.RequestPty("xterm", 80, 40, ssh.TerminalModes{ssh.ECHO: 0}); err != nil {
		return "", err
	}
	if err := sess.Shell(); err != nil {
		return "", err
	}

	time.Sleep(400 * time.Millisecond)

	// Fastpath (EdgeSwitch): terminal length 0 работает только в privileged (#).
	// До enable пейджер не выключается → show poe status all обрывается на "--More--"
	// и SFP/хвост портов не попадают в парсер (залипает старый poe_active в БД).
	var prefix []string
	if profile.privilegeCmd != "" {
		prefix = append(prefix, profile.privilegeCmd)
		if profile.needsEnablePass {
			prefix = append(prefix, enablePass)
		}
	}
	prefix = append(prefix, "terminal length 0")
	showCmd := "show poe status all"
	if len(profile.poeCommands) > 0 {
		showCmd = profile.poeCommands[0]
	}

	for _, s := range prefix {
		if _, err := in.Write([]byte(s + "\r\n")); err != nil {
			return "", err
		}
		time.Sleep(350 * time.Millisecond)
	}
	if _, err := in.Write([]byte(showCmd + "\r\n")); err != nil {
		return "", err
	}
	waitPoeDump(in, &outBuf, 800*time.Millisecond, 45*time.Second)
	_, _ = in.Write([]byte("exit\r\n"))
	_ = in.Close()
	waitErr := sess.Wait()
	combined := stripPoePager(outBuf.String() + "\n" + errBuf.String())
	if strings.TrimSpace(combined) == "" {
		if waitErr != nil {
			return "", waitErr
		}
		return "", fmt.Errorf("ssh poe: empty CLI output")
	}
	if waitErr != nil {
		if strings.Contains(strings.ToLower(waitErr.Error()), "without exit status") {
			return combined, nil
		}
		// Вывод есть — отдаём парсеру даже при странном exit.
		return combined, nil
	}
	return combined, nil
}

func waitPoeDump(in interface{ Write([]byte) (int, error) }, buf *bytes.Buffer, quiet, max time.Duration) {
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
		if in != nil && poeTailHasPager(buf.String()) && time.Since(lastChange) >= 80*time.Millisecond {
			if lastSpace.IsZero() || time.Since(lastSpace) >= 80*time.Millisecond {
				_, _ = in.Write([]byte(" "))
				lastSpace = time.Now()
			}
			time.Sleep(80 * time.Millisecond)
			continue
		}
		if n > 0 && time.Since(lastChange) >= quiet && !poeTailHasPager(buf.String()) {
			return
		}
		time.Sleep(80 * time.Millisecond)
	}
}

func poeTailHasPager(s string) bool {
	if len(s) > 80 {
		s = s[len(s)-80:]
	}
	l := strings.ToLower(s)
	return strings.Contains(l, "--more--") || strings.Contains(l, "(q)uit")
}

var poePagerMoreRe = regexp.MustCompile(`(?i)--More--[^\r\n]*`)

func stripPoePager(s string) string {
	s = poePagerMoreRe.ReplaceAllString(s, "")
	// Хвост " or (q)uit" / артефакты пробелов пейджера на отдельной строке.
	var b strings.Builder
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.EqualFold(t, "or (q)uit") || strings.EqualFold(t, "(q)uit") {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

func looksLikeUnsupportedCommand(s string) bool {
	l := strings.ToLower(s)
	return strings.Contains(l, "invalid input") ||
		strings.Contains(l, "unknown command") ||
		strings.Contains(l, "incomplete command") ||
		strings.Contains(l, "unrecognized command")
}

func parseUbiquitiShowPoeStatusToIfIndex(out string, ifRows map[int]snmp.IfRow) map[int]bool {
	byName := make(map[string]int)
	for ifIdx, row := range ifRows {
		name := strings.TrimSpace(row.IfName)
		if name != "" {
			byName[strings.ToLower(name)] = ifIdx
		}
	}

	res := make(map[int]bool)
	for _, line := range strings.Split(out, "\n") {
		// Формат "show poe status all":
		// Intf  Detection  Class  Consumed(W) Voltage(V) Current(mA) ...
		if m := poeStatusAllLineRe.FindStringSubmatch(line); len(m) == 9 {
			port := strings.ToLower(strings.TrimSpace(m[1]))
			detection := strings.ToLower(strings.TrimSpace(m[2]))
			consumedW := parsePoEFloat(m[4])
			voltageV := parsePoEFloat(m[5])
			currentMA := parsePoEFloat(m[6])

			ifIdx := byName[port]
			if ifIdx <= 0 {
				continue
			}
			if isPoEActiveFromDetailedColumns(detection, consumedW, voltageV, currentMA) {
				res[ifIdx] = true
			} else {
				res[ifIdx] = false
			}
			continue
		}
		// Формат SNR "show power inline":
		// Ethernet1/0/37 enable on 2900 class 33000 48 55 low 3
		if m := snrPowerInlineLineRe.FindStringSubmatch(strings.ToLower(line)); len(m) == 4 {
			port := strings.TrimSpace(m[1]) // ethernet1/0/37
			oper := strings.TrimSpace(m[2]) // on/off
			powerMW := parsePoEFloat(m[3])
			ifIdx := byName[port]
			if ifIdx <= 0 {
				continue
			}
			if oper == "on" || powerMW > 0 {
				res[ifIdx] = true
			} else {
				res[ifIdx] = false
			}
			continue
		}

		m := portLineRe.FindStringSubmatch(line)
		if len(m) != 3 {
			continue
		}
		port := strings.ToLower(strings.TrimSpace(m[1]))
		rest := strings.ToLower(strings.TrimSpace(m[2]))

		ifIdx := byName[port]
		if ifIdx <= 0 {
			continue
		}

		if isPoEDeliveringState(rest) {
			res[ifIdx] = true
		} else {
			res[ifIdx] = false
		}
	}
	return res
}

func isPoEDeliveringState(s string) bool {
	if strings.Contains(s, "delivering") ||
		strings.Contains(s, "power on") ||
		strings.Contains(s, "powered") {
		return true
	}
	// На некоторых CLI мощность уже в колонке без явного "delivering": >0 считаем активной выдачей.
	for _, tok := range strings.Fields(strings.ReplaceAll(s, ",", ".")) {
		if v, err := strconv.ParseFloat(strings.TrimSuffix(tok, "w"), 64); err == nil && v > 0 {
			return true
		}
	}
	return false
}

func parsePoEFloat(s string) float64 {
	v, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(strings.ToLower(s)), ",", "."), 64)
	if err != nil {
		return 0
	}
	return v
}

func isPoEActiveFromDetailedColumns(detection string, consumedW, voltageV, currentMA float64) bool {
	if strings.Contains(detection, "good") ||
		strings.Contains(detection, "delivering") ||
		strings.Contains(detection, "powered") {
		return true
	}
	// Напряжение на PSE часто есть и без PD (Searching) — по voltage не судим.
	// Ток/потребление > 0 — фактическая выдача.
	_ = voltageV
	return consumedW > 0 || currentMA > 0
}
