package store

import "strings"

// Режимы ignore из UI (цикл: off → soft → all → off).
const (
	IgnoreModeOff  = "off"
	IgnoreModeSoft = "soft"
	IgnoreModeAll  = "all"

	ignoreCommentSoft = "UI:ignore_soft"
	ignoreCommentAll  = "UI:ignore_all"
)

// IgnoreSoftEventTypes — все типы событий poller, кроме MAC/интрузии (UNKNOWN_MAC, MAC_MOVED).
const IgnoreSoftEventTypes = "LINK_DOWN,LINK_UP,PORT_UTILIZATION_HIGH,PORT_UTILIZATION_OK,PORT_SPEED_DOWN,PORT_SPEED_OK,MAC_REMOVED,ACCESS_PORT_LONG_IDLE_DEVICE,ACCESS_PORT_MAC_SUBSTITUTED"

// PortIgnoreFromMode строит запись для upsert; для off возвращает nil.
func PortIgnoreFromMode(deviceID int64, ifIndex int, mode string) *PortEventIgnore {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case IgnoreModeSoft:
		et := IgnoreSoftEventTypes
		c := ignoreCommentSoft
		return &PortEventIgnore{
			DeviceID: deviceID, IfIndex: ifIndex,
			EventTypes: &et,
			BlockEvents: false, BlockNotify: true, BlockActions: true,
			Comment: &c,
		}
	case IgnoreModeAll:
		c := ignoreCommentAll
		return &PortEventIgnore{
			DeviceID: deviceID, IfIndex: ifIndex,
			EventTypes: nil,
			BlockEvents: true, BlockNotify: true, BlockActions: true,
			Comment: &c,
		}
	default:
		return nil
	}
}

// ClassifyPortIgnoreMode определяет режим по сохранённой записи.
func ClassifyPortIgnoreMode(r PortEventIgnore) string {
	if c := r.Comment; c != nil {
		switch strings.TrimSpace(*c) {
		case ignoreCommentAll:
			if r.BlockEvents && r.BlockNotify && r.BlockActions {
				return IgnoreModeAll
			}
		case ignoreCommentSoft:
			return IgnoreModeSoft
		}
	}
	if r.BlockEvents && r.BlockNotify && r.BlockActions && matchesAllEventTypes(r.EventTypes) {
		return IgnoreModeAll
	}
	if r.BlockNotify && r.BlockActions && !r.BlockEvents {
		et := ""
		if r.EventTypes != nil {
			et = strings.TrimSpace(*r.EventTypes)
		}
		if et == IgnoreSoftEventTypes {
			return IgnoreModeSoft
		}
	}
	// Старая запись без комментария UI: block_events=false, все типы → soft-подобно.
	if r.BlockNotify && r.BlockActions {
		if matchesAllEventTypes(r.EventTypes) && !r.BlockEvents {
			return IgnoreModeSoft
		}
		if matchesAllEventTypes(r.EventTypes) && r.BlockEvents {
			return IgnoreModeAll
		}
	}
	return IgnoreModeSoft
}

func matchesAllEventTypes(filter *string) bool {
	if filter == nil {
		return true
	}
	raw := strings.TrimSpace(*filter)
	return raw == "" || raw == "*"
}
