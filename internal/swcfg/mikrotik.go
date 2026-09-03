package swcfg

import (
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

func mikrotikEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, `$`, `\$`)
	return s
}

// mikrotikIfaceQuote — имя интерфейса в RouterOS find (кавычки и экранирование).
func mikrotikIfaceQuote(iface string) string {
	return `"` + mikrotikEscape(strings.TrimSpace(iface)) + `"`
}

func mikrotikCommentValue(raw string) string {
	d := strings.TrimSpace(raw)
	d = strings.ReplaceAll(d, "\r", " ")
	d = strings.ReplaceAll(d, "\n", " ")
	for strings.Contains(d, "  ") {
		d = strings.ReplaceAll(d, "  ", " ")
	}
	return `"` + mikrotikEscape(d) + `"`
}

// MikrotikPortCmds строит команды RouterOS для изменения порта (без interactive configure).
func MikrotikPortCmds(iface string, ch PortChange) ([]string, error) {
	iface = strings.TrimSpace(iface)
	if iface == "" {
		return nil, fmt.Errorf("пустое имя интерфейса")
	}
	if ch.Isolate != nil {
		return nil, fmt.Errorf("MikroTik: изоляция порта пока не поддерживается")
	}
	if ch.FlowControl != nil {
		return nil, fmt.Errorf("MikroTik: flow control через CLI пока не поддерживается")
	}
	if ch.STP != nil {
		return nil, fmt.Errorf("MikroTik: STP через CLI пока не поддерживается")
	}
	q := mikrotikIfaceQuote(iface)
	find := "[find name=" + q + "]"
	var cmds []string
	if ch.AdminUp != nil {
		if *ch.AdminUp {
			cmds = append(cmds, "/interface set "+find+" disabled=no")
		} else {
			cmds = append(cmds, "/interface set "+find+" disabled=yes")
		}
	}
	if ch.Description != nil {
		d := strings.TrimSpace(*ch.Description)
		if d == "" {
			cmds = append(cmds, "/interface set "+find+" comment=\"\"")
		} else {
			cmds = append(cmds, "/interface set "+find+" comment="+mikrotikCommentValue(d))
		}
	}
	if ch.PoEMode != nil {
		poe, err := PoEModeCLI(VendorMikrotik, *ch.PoEMode)
		if err != nil {
			return nil, err
		}
		// poe строка уже содержит полный путь команды с плейсхолдером %s или find.
		cmds = append(cmds, fmt.Sprintf(poe, find))
	}
	if len(cmds) == 0 {
		return nil, fmt.Errorf("нечего менять")
	}
	return cmds, nil
}

func applyMikrotikPortChange(c Creds, ch PortChange) error {
	cmds, err := MikrotikPortCmds(ch.Interface, ch)
	if err != nil {
		return err
	}
	host := strings.TrimSpace(c.Host)
	user := strings.TrimSpace(c.User)
	if host == "" || user == "" {
		return fmt.Errorf("нет host или ssh user")
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
	cfg := switchSSHConfig(user, c.Password, timeout, hk)
	addr := fmt.Sprintf("%s:%d", host, port)
	client, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return fmt.Errorf("ssh %s: %w", host, err)
	}
	defer client.Close()

	// Один remote command: RouterOS выполняет цепочку через ;
	script := strings.Join(cmds, "; ")
	sess, err := client.NewSession()
	if err != nil {
		return err
	}
	out, runErr := sess.CombinedOutput(script)
	_ = sess.Close()
	s := string(out)
	if ierr := interpretMikrotikCLI(s); ierr != nil {
		if runErr != nil {
			return fmt.Errorf("%w; %s", ierr, runErr)
		}
		return ierr
	}
	if runErr != nil && strings.TrimSpace(s) == "" {
		return runErr
	}
	return nil
}

func interpretMikrotikCLI(out string) error {
	low := strings.ToLower(out)
	for _, bad := range []string{
		"expected end of command",
		"syntax error",
		"bad command name",
		"no such item",
		"failure:",
		"not enough permissions",
		"permission denied",
		"invalid value",
	} {
		if strings.Contains(low, bad) {
			return fmt.Errorf("cli: %s — %s", bad, compactCLIErr(out))
		}
	}
	return nil
}

func fetchMikrotikExport(client *ssh.Client, timeout time.Duration) (string, error) {
	// Сначала non-interactive — надёжнее на RouterOS.
	for _, cmd := range []string{"/export compact", "/export"} {
		sess, err := client.NewSession()
		if err != nil {
			return "", err
		}
		done := make(chan struct{})
		var b []byte
		var runErr error
		go func() {
			b, runErr = sess.CombinedOutput(cmd)
			close(done)
		}()
		select {
		case <-done:
			_ = sess.Close()
			s := string(b)
			if runErr == nil && looksLikeMikrotikExport(s) {
				return s, nil
			}
		case <-time.After(timeout):
			_ = sess.Close()
		}
	}
	out, err := shellCommand(client, timeout, "/export compact")
	if err == nil && looksLikeMikrotikExport(out) {
		return out, nil
	}
	if err != nil {
		return "", err
	}
	return "", fmt.Errorf("MikroTik: не удалось снять /export")
}

func looksLikeMikrotikExport(s string) bool {
	if strings.Count(s, "\n") < 3 {
		return false
	}
	l := strings.ToLower(s)
	return strings.Contains(l, "/interface") ||
		strings.Contains(l, "/ip ") ||
		strings.Contains(l, "/system") ||
		strings.Contains(l, "routeros") ||
		(strings.Contains(l, "interface") && strings.Contains(l, "name="))
}
