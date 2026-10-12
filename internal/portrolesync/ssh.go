package portrolesync

import (
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/config"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/devssh"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/models"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

func ResolveDeviceSSH(dev *models.Device, bs store.BackupSettings, cfg config.Config) (user, pass, enable string, port int, timeout time.Duration) {
	return devssh.ResolveDevice(dev, bs, cfg)
}
