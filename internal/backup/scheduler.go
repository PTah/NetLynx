package backup

import (
	"context"
	"time"
)

func (r *Runner) Scheduler(ctx context.Context) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	var lastFired string
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			bs, err := r.st.GetBackupSettings(ctx)
			if err != nil || !bs.ScheduleEnabled {
				continue
			}
			if now.Hour() != bs.ScheduleHour || now.Minute() != bs.ScheduleMinute {
				continue
			}
			key := now.Format("2006-01-02-15-04")
			if lastFired == key {
				continue
			}
			lastFired = key
			r.log.Info("backup schedule start", "at", key)
			runCtx, cancel := context.WithTimeout(ctx, 90*time.Minute)
			err = r.RunNow(runCtx)
			cancel()
			if err != nil {
				r.log.Warn("backup schedule", "err", err)
			}
		}
	}
}
