package uisp

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

// RunOverviewSync периодически подтягивает overview.status коммутаторов из UISP и пишет в devices.uisp_overview_status.
// Для узлов без uisp_device_id поле не трогается; «онлайн» на дашборде для них по-прежнему по SNMP.
func RunOverviewSync(ctx context.Context, log *slog.Logger, st *store.Store, interval time.Duration) {
	if interval < 15*time.Second {
		interval = 15 * time.Second
	}
	syncOverviewOnce(ctx, log, st)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			syncOverviewOnce(ctx, log, st)
		}
	}
}

func syncOverviewOnce(ctx context.Context, log *slog.Logger, st *store.Store) {
	row, err := st.GetUISPSettings(ctx)
	if err != nil || !row.Enabled || row.BaseURL == nil || row.APIToken == nil {
		return
	}
	base := strings.TrimSpace(*row.BaseURL)
	tok := strings.TrimSpace(*row.APIToken)
	if base == "" || tok == "" {
		return
	}
	sc, cancel := context.WithTimeout(ctx, 2*time.Minute)
	m, err := FetchSwitchOverviewStatuses(sc, base, tok)
	cancel()
	if err != nil {
		log.Warn("uisp overview sync", "err", err)
		return
	}
	if _, err := st.ApplyUISPOverviewStatuses(ctx, m); err != nil {
		log.Warn("uisp overview apply", "err", err)
	}
}
