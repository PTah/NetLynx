package swcfg

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

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
