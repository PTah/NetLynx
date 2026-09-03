package swcfg

import (
	"fmt"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// sshAlgoProfile — набор алгоритмов для одного handshake. Пробуем по очереди:
// современные, затем переходные, затем legacy (Eltex OpenSSH 5.x).
type sshAlgoProfile struct {
	name             string
	keyExchanges     []string // nil = только дефолты crypto/ssh
	ciphers          []string
	macs             []string
	hostKeyAlgos     []string
}

func sshHostKeyProfiles() []sshAlgoProfile {
	return []sshAlgoProfile{
		{name: "modern"},
		{
			name: "transitional",
			keyExchanges: []string{
				"diffie-hellman-group-exchange-sha256",
				"diffie-hellman-group14-sha1",
			},
			hostKeyAlgos: []string{ssh.KeyAlgoRSA, ssh.KeyAlgoRSASHA256, ssh.KeyAlgoRSASHA512},
		},
		{
			name: "legacy",
			keyExchanges: []string{
				"diffie-hellman-group-exchange-sha1",
				"diffie-hellman-group14-sha1",
				"diffie-hellman-group1-sha1",
			},
			ciphers:      []string{"aes128-cbc", "aes192-cbc", "aes256-cbc", "3des-cbc"},
			macs:         []string{"hmac-sha1", "hmac-sha1-96"},
			hostKeyAlgos: []string{ssh.KeyAlgoRSA, ssh.KeyAlgoDSA},
		},
	}
}

func applyAlgoProfile(cfg *ssh.ClientConfig, p sshAlgoProfile) {
	cfg.SetDefaults()
	if len(p.keyExchanges) > 0 {
		cfg.KeyExchanges = append(append([]string{}, cfg.KeyExchanges...), p.keyExchanges...)
	}
	if len(p.ciphers) > 0 {
		cfg.Ciphers = append(append([]string{}, cfg.Ciphers...), p.ciphers...)
	}
	if len(p.macs) > 0 {
		cfg.MACs = append(append([]string{}, cfg.MACs...), p.macs...)
	}
	if len(p.hostKeyAlgos) > 0 {
		cfg.HostKeyAlgorithms = append(append([]string{}, cfg.HostKeyAlgorithms...), p.hostKeyAlgos...)
	}
}

// FetchHostKeyLine выполняет SSH-handshake (без логина) и возвращает строку known_hosts.
// Перебирает профили алгоритмов, пока пир не отдаст ключ — без группировки по вендору/подсети.
func FetchHostKeyLine(host string, port int, timeout time.Duration) (string, error) {
	line, _, err := FetchHostKeyLineWithProfile(host, port, timeout)
	return line, err
}

func FetchHostKeyLineWithProfile(host string, port int, timeout time.Duration) (line, profile string, err error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return "", "", fmt.Errorf("empty host")
	}
	if port <= 0 {
		port = 22
	}
	if timeout <= 0 {
		timeout = 12 * time.Second
	}

	var last error
	for _, p := range sshHostKeyProfiles() {
		line, err := fetchHostKeyOnce(host, port, timeout, p)
		if err == nil && strings.TrimSpace(line) != "" {
			return line, p.name, nil
		}
		last = err
	}
	if last == nil {
		last = fmt.Errorf("no host key from %s:%d", host, port)
	}
	return "", "", last
}

func fetchHostKeyOnce(host string, port int, timeout time.Duration, p sshAlgoProfile) (string, error) {
	var keyLine string
	hk := func(_ string, remote net.Addr, key ssh.PublicKey) error {
		addrs := []string{host}
		if port != 22 {
			addrs = append(addrs, fmt.Sprintf("[%s]:%d", host, port))
		}
		if remote != nil {
			if h, _, err := net.SplitHostPort(remote.String()); err == nil && h != "" {
				addrs = append(addrs, h)
			}
		}
		keyLine = knownhosts.Line(addrs, key)
		return nil
	}

	cfg := &ssh.ClientConfig{
		User:            "_hostkey_probe_",
		Auth:            []ssh.AuthMethod{ssh.Password("")},
		HostKeyCallback: hk,
		Timeout:         timeout,
	}
	applyAlgoProfile(cfg, p)

	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", host, port), cfg)
	if client != nil {
		_ = client.Close()
	}
	if keyLine != "" {
		return keyLine, nil
	}
	if err != nil {
		return "", err
	}
	return "", fmt.Errorf("no host key from %s:%d", host, port)
}
