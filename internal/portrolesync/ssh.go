package portrolesync

import (
	"time"

	"github.com/PTah/netlynx/internal/config"
	"github.com/PTah/netlynx/internal/devssh"
	"github.com/PTah/netlynx/internal/models"
	"github.com/PTah/netlynx/internal/store"
)

func ResolveDeviceSSH(dev *models.Device, bs store.BackupSettings, cfg config.Config) (user, pass, enable string, port int, timeout time.Duration) {
	return devssh.ResolveDevice(dev, bs, cfg)
}
