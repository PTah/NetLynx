package api

import (
	"net/http"
	"strings"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/live"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

// EventTypeConfigEdit — правка конфига свича/порта пользователем UI.
const EventTypeConfigEdit = "CONFIG_EDIT"

// emitConfigEditEvent пишет CONFIG_EDIT (warning) в общую таблицу events —
// событие видно и в глобальной ленте, и в карточке узла.
func (s *Server) emitConfigEditEvent(r *http.Request, deviceID int64, ifIndex *int, change string, details map[string]interface{}) {
	if deviceID <= 0 || change == "" {
		return
	}
	username, _ := r.Context().Value(authUserKey).(string)
	if username == "" {
		if s.cfg.AuthDisabled {
			username = "local"
		} else {
			username = "unknown"
		}
	}
	pl := map[string]interface{}{
		"username": username,
		"change":   change,
		"source":   "ui",
	}
	for k, v := range details {
		if k == "" || v == nil {
			continue
		}
		pl[k] = v
	}
	if ifIndex != nil && *ifIndex > 0 {
		if labels, ok, err := s.st.GetInterfaceEventLabels(r.Context(), deviceID, *ifIndex); err == nil && ok {
			store.ApplyEventIfaceLabels(pl, labels)
		}
	}
	id, err := s.st.InsertEvent(r.Context(), deviceID, ifIndex, EventTypeConfigEdit, "warning", pl)
	if err != nil {
		return
	}
	if s.hub == nil {
		return
	}
	name, host := "", ""
	if d, err := s.st.GetDevice(r.Context(), deviceID); err == nil && d != nil {
		name = d.Name
		host = strings.TrimSpace(d.Host)
	}
	s.hub.Publish(live.EventPayload{
		EventID:    id,
		DeviceID:   deviceID,
		DeviceName: name,
		DeviceHost: host,
		IfIndex:    ifIndex,
		EventType:  EventTypeConfigEdit,
		Severity:   "warning",
		Payload:    pl,
	})
}
