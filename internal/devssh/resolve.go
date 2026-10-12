package devssh

import (
	"strings"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/config"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/models"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

func ResolveDevice(dev *models.Device, bs store.BackupSettings, cfg config.Config) (user, pass, enable string, port int, timeout time.Duration) {
	user = strings.TrimSpace(derefStr(dev.SSHUser))
	pass = derefStr(dev.SSHPassword)
	enable = derefStr(dev.SSHEnablePassword)
	port = 22
	if bs.SSHPort > 0 {
		port = bs.SSHPort
	}
	if dev.SSHPort != nil && *dev.SSHPort > 0 {
		port = *dev.SSHPort
	}
	if user == "" {
		user = strings.TrimSpace(derefStr(bs.SSHUser))
	}
	if strings.TrimSpace(pass) == "" {
		pass = derefStr(bs.SSHPassword)
	}
	if strings.TrimSpace(enable) == "" {
		enable = derefStr(bs.SSHEnablePassword)
	}
	if user == "" {
		user = strings.TrimSpace(cfg.SSHPOEUser)
	}
	if strings.TrimSpace(pass) == "" {
		pass = cfg.SSHPOEPass
	}
	if strings.TrimSpace(enable) == "" {
		enable = cfg.SSHPOEEnablePass
		if enable == "" {
			enable = pass
		}
	}
	if port <= 0 {
		port = 22
	}
	if cfg.SSHPOEPort > 0 && (dev.SSHPort == nil || *dev.SSHPort <= 0) && bs.SSHPort <= 0 {
		port = cfg.SSHPOEPort
	}
	timeout = 30 * time.Second
	if bs.SSHTimeoutSeconds >= 5 {
		timeout = time.Duration(bs.SSHTimeoutSeconds) * time.Second
	}
	if cfg.SSHPOETimeout > 0 && timeout < cfg.SSHPOETimeout {
		timeout = cfg.SSHPOETimeout
	}
	if timeout < 15*time.Second {
		timeout = 15 * time.Second
	}
	return user, pass, enable, port, timeout
}

func KnownHostsPath(cfg config.Config) string {
	if strings.TrimSpace(cfg.SSHPOEKnownHosts) != "" {
		return cfg.SSHPOEKnownHosts
	}
	return "/var/lib/netlynx/ssh_known_hosts"
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
