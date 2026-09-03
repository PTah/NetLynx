package swcfg

import (
	"time"

	"golang.org/x/crypto/ssh"
)

func switchSSHConfig(user, password string, timeout time.Duration, hk ssh.HostKeyCallback) *ssh.ClientConfig {
	cfg := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.KeyboardInteractive(kbPassword(password)),
			ssh.Password(password),
		},
		HostKeyCallback: hk,
		Timeout:         timeout,
	}
	cfg.SetDefaults()
	cfg.KeyExchanges = append(cfg.KeyExchanges,
		"diffie-hellman-group-exchange-sha256",
		"diffie-hellman-group-exchange-sha1",
		"diffie-hellman-group1-sha1",
	)
	cfg.Ciphers = append(cfg.Ciphers, "aes128-cbc", "aes192-cbc", "aes256-cbc", "3des-cbc")
	cfg.MACs = append(cfg.MACs, "hmac-sha1", "hmac-sha1-96")
	cfg.HostKeyAlgorithms = append(cfg.HostKeyAlgorithms,
		ssh.KeyAlgoRSA,
		ssh.KeyAlgoRSASHA256,
		ssh.KeyAlgoRSASHA512,
		ssh.KeyAlgoDSA,
	)
	return cfg
}

func kbPassword(password string) ssh.KeyboardInteractiveChallenge {
	return func(user, instruction string, questions []string, echos []bool) ([]string, error) {
		_ = user
		_ = instruction
		_ = echos
		ans := make([]string, len(questions))
		for i := range questions {
			ans[i] = password
		}
		return ans, nil
	}
}
