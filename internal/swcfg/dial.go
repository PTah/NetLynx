package swcfg

import (
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// DialSwitch — SSH к свитчу: перебор algo-профилей, при hostkey/RSA — refresh known_hosts и один повтор.
func DialSwitch(c Creds) (*ssh.Client, error) {
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
	addr := fmt.Sprintf("%s:%d", host, port)

	client, err := dialSwitchProfiles(c, user, addr, timeout)
	if err == nil {
		return client, nil
	}
	if !isHostKeyRecoverable(err) {
		return nil, fmt.Errorf("ssh %s: %w", host, err)
	}
	if rerr := RefreshHostKey(c.KnownHosts, host, port, timeout); rerr != nil {
		return nil, fmt.Errorf("ssh %s: %w (host key refresh: %v)", host, err, rerr)
	}
	client, err2 := dialSwitchProfiles(c, user, addr, timeout)
	if err2 == nil {
		return client, nil
	}
	return nil, fmt.Errorf("ssh %s: %w", host, err2)
}

func dialSwitchProfiles(c Creds, user, addr string, timeout time.Duration) (*ssh.Client, error) {
	hk, err := HostKeyCallback(c.KnownHosts)
	if err != nil {
		return nil, err
	}
	profiles := append([]sshAlgoProfile{{name: "compat"}}, sshHostKeyProfiles()...)
	var last error
	for _, p := range profiles {
		cfg := switchSSHConfig(user, c.Password, timeout, hk)
		if p.name != "compat" {
			applyAlgoProfile(cfg, p)
		}
		client, derr := ssh.Dial("tcp", addr, cfg)
		if derr == nil {
			return client, nil
		}
		last = derr
		if ClassifySSHError(derr) == "auth" {
			return nil, derr
		}
	}
	if last == nil {
		last = fmt.Errorf("ssh dial failed")
	}
	return nil, last
}

// ClassifySSHError — auth | hostkey | timeout | other.
func ClassifySSHError(err error) string {
	if err == nil {
		return ""
	}
	s := strings.ToLower(err.Error())
	switch {
	case strings.Contains(s, "unable to authenticate"),
		strings.Contains(s, "no supported methods remain"),
		strings.Contains(s, "permission denied"),
		strings.Contains(s, "authentication failed"):
		return "auth"
	case strings.Contains(s, "crypto/rsa"),
		strings.Contains(s, "ключ хоста"),
		strings.Contains(s, "host key"),
		strings.Contains(s, "knownhosts"),
		strings.Contains(s, "unable to authenticate host"):
		return "hostkey"
	case strings.Contains(s, "timeout"),
		strings.Contains(s, "i/o timeout"),
		strings.Contains(s, "deadline exceeded"):
		return "timeout"
	default:
		return "other"
	}
}

func isHostKeyRecoverable(err error) bool {
	if err == nil {
		return false
	}
	if ClassifySSHError(err) == "auth" {
		return false
	}
	s := strings.ToLower(err.Error())
	if strings.Contains(s, "возможна подмена") {
		return true
	}
	return ClassifySSHError(err) == "hostkey" || strings.Contains(s, "crypto/rsa")
}
