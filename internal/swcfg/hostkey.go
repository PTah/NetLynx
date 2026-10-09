package swcfg

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

var knownHostsMu sync.Mutex

// HostKeyCallback принимает неизвестный ключ (как «yes» при первом SSH) и пишет его в known_hosts.
// Повторный коннект к тому же хосту с другим ключом — ошибка.
func HostKeyCallback(knownHostsPath string) (ssh.HostKeyCallback, error) {
	path := strings.TrimSpace(knownHostsPath)
	if path == "" {
		path = "/var/lib/netlynx/ssh_known_hosts"
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
		return nil, fmt.Errorf("known_hosts dir: %w", err)
	}
	f, err := os.OpenFile(abs, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("known_hosts %s: %w", abs, err)
	}
	_ = f.Close()
	_ = os.Chmod(abs, 0o600)

	inner, err := knownhosts.New(abs)
	if err != nil {
		return nil, fmt.Errorf("known_hosts: %w", err)
	}
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := inner(hostname, remote, key)
		if err == nil {
			return nil
		}
		ke, ok := err.(*knownhosts.KeyError)
		if !ok {
			return err
		}
		if len(ke.Want) > 0 {
			if hostKeySameTypeMismatch(ke.Want, key) {
				return fmt.Errorf("SSH: ключ хоста %s изменился (возможна подмена)", hostname)
			}
			// Тот же хост, другой тип ключа (rsa vs ecdsa) — дописываем, как OpenSSH.
			if err := appendKnownHost(abs, hostname, remote, key); err != nil {
				return err
			}
			return nil
		}
		if err := appendKnownHost(abs, hostname, remote, key); err != nil {
			return err
		}
		return nil
	}, nil
}

func appendKnownHost(path, hostname string, remote net.Addr, key ssh.PublicKey) error {
	knownHostsMu.Lock()
	defer knownHostsMu.Unlock()
	addrs := []string{hostname}
	if remote != nil {
		addrs = append(addrs, remote.String())
	}
	line := knownhosts.Line(addrs, key)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintln(f, line)
	return err
}

func hostKeySameTypeMismatch(want []knownhosts.KnownKey, key ssh.PublicKey) bool {
	if key == nil {
		return true
	}
	for _, k := range want {
		if k.Key != nil && k.Key.Type() == key.Type() {
			return true
		}
	}
	return false
}

// RefreshHostKey — probe ключа (все algo-профили) и замена записей хоста в known_hosts.
func RefreshHostKey(knownHostsPath, host string, port int, timeout time.Duration) error {
	line, _, err := FetchHostKeyLineWithProfile(host, port, timeout)
	if err != nil {
		return err
	}
	return ReplaceHostKeyLines(knownHostsPath, host, port, line)
}

// EnsureHostKey — TOFU: если хоста ещё нет в known_hosts, probe и дописать.
func EnsureHostKey(knownHostsPath, host string, port int, timeout time.Duration) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return fmt.Errorf("empty host")
	}
	path := strings.TrimSpace(knownHostsPath)
	if path == "" {
		path = "/var/lib/netlynx/ssh_known_hosts"
	}
	if hostKnownInFile(path, host, port) {
		return nil
	}
	return RefreshHostKey(path, host, port, timeout)
}

// ReplaceHostKeyLines удаляет старые строки хоста и дописывает новую (одна строка known_hosts).
func ReplaceHostKeyLines(knownHostsPath, host string, port int, newLine string) error {
	path := strings.TrimSpace(knownHostsPath)
	if path == "" {
		path = "/var/lib/netlynx/ssh_known_hosts"
	}
	newLine = strings.TrimSpace(newLine)
	if newLine == "" {
		return fmt.Errorf("empty host key line")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
		return fmt.Errorf("known_hosts dir: %w", err)
	}

	knownHostsMu.Lock()
	defer knownHostsMu.Unlock()

	var kept []string
	if b, err := os.ReadFile(abs); err == nil {
		sc := bufio.NewScanner(strings.NewReader(string(b)))
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				kept = append(kept, sc.Text())
				continue
			}
			if knownHostsLineMatchesHost(line, host, port) {
				continue
			}
			kept = append(kept, sc.Text())
		}
	}
	kept = append(kept, newLine)
	tmp := abs + ".tmp"
	data := strings.Join(kept, "\n") + "\n"
	if err := os.WriteFile(tmp, []byte(data), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, abs); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	_ = os.Chmod(abs, 0o600)
	return nil
}

func hostKnownInFile(path, host string, port int) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	sc := bufio.NewScanner(strings.NewReader(string(b)))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if knownHostsLineMatchesHost(line, host, port) {
			return true
		}
	}
	return false
}

func knownHostsLineMatchesHost(line, host string, port int) bool {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return false
	}
	markers := hostMarkers(host, port)
	for _, part := range strings.Split(fields[0], ",") {
		p := strings.TrimSpace(part)
		for _, m := range markers {
			if p == m {
				return true
			}
		}
	}
	return false
}

func hostMarkers(host string, port int) []string {
	host = strings.TrimSpace(host)
	out := []string{host}
	if port > 0 && port != 22 {
		out = append(out, fmt.Sprintf("[%s]:%d", host, port))
	}
	return out
}
