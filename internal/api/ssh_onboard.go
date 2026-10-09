package api

import (
	"context"

	"github.com/PTah/netlynx/internal/configssh"
)

// SetSSHReporter подключает алерты CONFIG_SSH_FAIL и onboard-probe.
func (s *Server) SetSSHReporter(rep *configssh.Reporter) {
	s.sshReporter = rep
	if s.backupRun != nil {
		s.backupRun.SetReporter(rep)
	}
}

func (s *Server) onboardSSHDevice(ctx context.Context, deviceID int64) string {
	if s.sshReporter == nil || deviceID <= 0 {
		return ""
	}
	dev, err := s.st.GetDevice(ctx, deviceID)
	if err != nil || dev == nil {
		return ""
	}
	return s.sshReporter.OnboardDevice(ctx, dev)
}
