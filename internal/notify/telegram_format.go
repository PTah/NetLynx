package notify

import (
	"fmt"
	"strings"
	"time"
)

func escapeTelegramHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func pluralWordRU(n int, one, few, many string) string {
	abs := n % 100
	if abs >= 11 && abs <= 14 {
		return many
	}
	switch n % 10 {
	case 1:
		return one
	case 2, 3, 4:
		return few
	default:
		return many
	}
}

func formatTelegramDurationRU(sec int) string {
	if sec < 0 {
		sec = 0
	}
	h := sec / 3600
	m := (sec % 3600) / 60
	var parts []string
	if h > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", h, pluralWordRU(h, "час", "часа", "часов")))
	}
	if m > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", m, pluralWordRU(m, "минута", "минуты", "минут")))
	}
	if len(parts) == 0 {
		return "менее минуты"
	}
	return strings.Join(parts, " ")
}

func telegramClock(t time.Time) string {
	return t.Format("02/01/2006 15:04:05")
}

func telegramDeviceLine(category, name, host string) string {
	noun := deviceNounRU(category)
	name = strings.TrimSpace(name)
	host = strings.TrimSpace(host)
	if name == "" {
		name = host
	}
	line := noun + ": " + escapeTelegramHTML(name)
	if host != "" && host != name {
		line += " (" + escapeTelegramHTML(host) + ")"
	}
	return line
}

func telegramEventLine(eventType string, payload map[string]interface{}, at time.Time) string {
	et := strings.ToUpper(strings.TrimSpace(eventType))
	switch et {
	case "DEVICE_OFFLINE":
		return "<b><u>DEVICE OFFLINE</u></b> с " + telegramClock(payloadEventClock(eventType, payload, at))
	case "DEVICE_ONLINE":
		return "<b><u>DEVICE ONLINE</u></b> с " + telegramClock(at.Local())
	default:
		line := escapeTelegramHTML(strings.TrimSpace(eventType))
		if mac, ok := payload["mac"].(string); ok && strings.TrimSpace(mac) != "" {
			line += " · " + escapeTelegramHTML(strings.TrimSpace(mac))
		}
		return line
	}
}

func formatTelegramFromCard(card EmailDeviceCard, deviceHost string, eventType string, payload map[string]interface{}, at time.Time) string {
	if at.IsZero() {
		at = time.Now()
	}
	host := strings.TrimSpace(deviceHost)
	if host == "" {
		host = strings.TrimSpace(card.Subtitle)
	}
	et := strings.ToUpper(strings.TrimSpace(eventType))
	var b strings.Builder
	b.WriteString("<b>NetLynx</b>\n")
	b.WriteString(telegramEventLine(eventType, payload, at))
	b.WriteByte('\n')
	if et == "DEVICE_ONLINE" {
		if dur := payloadOfflineDurationSec(payload); dur > 0 {
			b.WriteString("(время оффлайна: " + formatTelegramDurationRU(dur) + ")\n")
		}
	}
	b.WriteString(telegramDeviceLine(card.Category, card.Title, host))
	att := strings.TrimSpace(card.Attachment)
	if att != "" {
		b.WriteByte('\n')
		b.WriteString("(" + escapeTelegramHTML("Закреплено "+att) + ")")
	}
	return b.String()
}
