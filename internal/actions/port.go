package actions

import (
	"context"
	"log/slog"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/snmp"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

// TryPortAdminDown выключает порт administratively down через IF-MIB SET (ifAdminStatus=2).
func TryPortAdminDown(ctx context.Context, log *slog.Logger, st *store.Store, dev store.PollDevice, ifIndex int) error {
	if ifIndex <= 0 {
		return nil
	}
	g, err := snmp.NewGoSNMP(dev)
	if err != nil {
		return err
	}
	if err := g.Connect(); err != nil {
		return err
	}
	defer g.Conn.Close()
	if err := snmp.SetIfAdminStatus(g, ifIndex, 2); err != nil {
		return err
	}
	pl := map[string]interface{}{"if_index": ifIndex, "action": "admin_down"}
	_, err = st.InsertEvent(ctx, dev.ID, &ifIndex, "PORT_ADMIN_DOWN_ACTION", "warning", pl)
	if log != nil {
		log.Info("incident action port admin down", "device_id", dev.ID, "if_index", ifIndex)
	}
	return err
}
