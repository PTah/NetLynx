package notify

import (
	"bytes"
	_ "embed"
	"fmt"
	"html"
	"strings"
	"time"
)

//go:embed assets/logo.png
var embeddedLogoPNG []byte

// EmailDeviceCard одна строка/карточка устройства в письме.
type EmailDeviceCard struct {
	DeviceID   int64
	Title      string // имя устройства (главное)
	Subtitle   string // host / IP · тип
	Detail     string // расположение (опционально)
	Attachment string // «на коммутаторе X, порт Y» (для не-свитчей)
	StatusLine string // статус оффлайн/онлайн
	Kind               string // DEVICE_OFFLINE | DEVICE_ONLINE | other
	Category           string // switch | camera | …
	URL                string // ссылка на карточку
	OfflineDurationSec int    // для DEVICE_ONLINE: не гасить OFFLINE в пачке, если простой дольше окна
}

// EmailAlertMail готовое письмо (текст + HTML).
type EmailAlertMail struct {
	Subject  string
	TextBody string
	HTMLBody string
}

func LogoPNG() []byte { return embeddedLogoPNG }

func buildDeviceURL(base string, deviceID int64) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" || deviceID <= 0 {
		return ""
	}
	return fmt.Sprintf("%s/devices/%d", base, deviceID)
}

func greetingFromTo(toCSV string) string {
	parts := splitCSV(toCSV)
	if len(parts) == 0 {
		return "Здравствуйте"
	}
	addr := parts[0]
	if i := strings.IndexByte(addr, '@'); i > 0 {
		local := addr[:i]
		if local != "" {
			return "Здравствуйте, " + local
		}
	}
	return "Здравствуйте"
}

func formatOfflineDuration(sec int) string {
	if sec < 0 {
		sec = 0
	}
	h := sec / 3600
	m := (sec % 3600) / 60
	s := sec % 60
	if h > 0 {
		return fmt.Sprintf("%dч %dм %dс", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dм %dс", m, s)
	}
	return fmt.Sprintf("%dс", s)
}

func payloadReason(payload map[string]interface{}) string {
	if payload == nil {
		return ""
	}
	v, ok := payload["reason"]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "snmp":
		return "нет ответа SNMP"
	case "ping":
		return "нет ответа ping"
	case "reachability":
		return "нет связи"
	default:
		return strings.TrimSpace(s)
	}
}

func payloadOfflineDurationSec(payload map[string]interface{}) int {
	if payload == nil {
		return 0
	}
	v, ok := payload["offline_duration_sec"]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case int32:
		return int(n)
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

func payloadEventClock(eventType string, payload map[string]interface{}, at time.Time) time.Time {
	at = at.Local()
	if strings.ToUpper(strings.TrimSpace(eventType)) != "DEVICE_OFFLINE" {
		return at
	}
	for _, k := range []string{"offline_since", "was_offline_since"} {
		s, _ := payload[k].(string)
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			continue
		}
		return t.In(at.Location())
	}
	return at
}

func cardStatusLine(eventType, category string, payload map[string]interface{}, at time.Time) string {
	clock := payloadEventClock(eventType, payload, at).Format("15:04")
	reason := payloadReason(payload)
	noun := deviceNounRU(category)
	switch strings.ToUpper(strings.TrimSpace(eventType)) {
	case "DEVICE_OFFLINE":
		if reason != "" {
			return fmt.Sprintf("%s ушёл в оффлайн в %s (%s)", noun, clock, reason)
		}
		return fmt.Sprintf("%s ушёл в оффлайн в %s", noun, clock)
	case "DEVICE_ONLINE":
		dur := ""
		if v, ok := payload["offline_duration_sec"]; ok {
			switch n := v.(type) {
			case int:
				dur = formatOfflineDuration(n)
			case int64:
				dur = formatOfflineDuration(int(n))
			case float64:
				dur = formatOfflineDuration(int(n))
			}
		}
		if dur != "" {
			return fmt.Sprintf("%s снова в сети с %s после %s отсутствия в сети", noun, clock, dur)
		}
		return fmt.Sprintf("%s снова в сети с %s", noun, clock)
	default:
		return strings.TrimSpace(eventType) + " · " + clock
	}
}

func deviceNounRU(category string) string {
	switch strings.ToLower(strings.TrimSpace(category)) {
	case "switch":
		return "Коммутатор"
	case "router":
		return "Роутер"
	case "ap":
		return "Точка доступа"
	case "server":
		return "Сервер"
	case "computer":
		return "Компьютер"
	case "phone":
		return "Телефон"
	case "mfu":
		return "МФУ"
	case "camera":
		return "Камера"
	default:
		return "Устройство"
	}
}

func introForCards(cards []EmailDeviceCard) string {
	offline, online, other := 0, 0, 0
	for _, c := range cards {
		switch strings.ToUpper(c.Kind) {
		case "DEVICE_OFFLINE":
			offline++
		case "DEVICE_ONLINE":
			online++
		default:
			other++
		}
	}
	n := len(cards)
	switch {
	case offline > 0 && online == 0 && other == 0:
		if n == 1 {
			c := cards[0]
			name := c.Title
			if name == "" {
				name = "устройство"
			}
			cat := strings.ToLower(strings.TrimSpace(c.Category))
			var msg string
			switch cat {
			case "switch", "router":
				msg = fmt.Sprintf("%s «%s» ушёл в оффлайн.", strings.ToLower(deviceNounRU(c.Category)), name)
			default:
				if c.Subtitle != "" {
					msg = fmt.Sprintf("Устройство «%s» (%s) ушло в оффлайн.", name, c.Subtitle)
				} else {
					msg = fmt.Sprintf("Устройство «%s» ушло в оффлайн.", name)
				}
			}
			if c.Attachment != "" {
				msg += " Закреплено " + c.Attachment + "."
			}
			return msg
		}
		return fmt.Sprintf("%d устройств из списка узлов ушли в оффлайн.", n)
	case online > 0 && offline == 0 && other == 0:
		if n == 1 {
			c := cards[0]
			name := c.Title
			if name == "" {
				name = "устройство"
			}
			cat := strings.ToLower(strings.TrimSpace(c.Category))
			switch cat {
			case "switch", "router":
				return fmt.Sprintf("%s «%s» снова в сети.", strings.ToLower(deviceNounRU(c.Category)), name)
			default:
				return fmt.Sprintf("Устройство «%s» снова в сети.", name)
			}
		}
		return fmt.Sprintf("%d устройств снова в сети.", n)
	case n == 1:
		return "новое событие по устройству в NetLynx."
	default:
		return fmt.Sprintf("%d событий по устройствам в NetLynx.", n)
	}
}

func subjectForCards(cards []EmailDeviceCard) string {
	if len(cards) == 0 {
		return "NetLynx Оповещение"
	}
	nameOf := func(c EmailDeviceCard) string {
		if strings.TrimSpace(c.Title) != "" {
			return c.Title
		}
		return c.Subtitle
	}
	offline, online := 0, 0
	for _, c := range cards {
		switch strings.ToUpper(c.Kind) {
		case "DEVICE_OFFLINE":
			offline++
		case "DEVICE_ONLINE":
			online++
		}
	}
	if offline > 0 && online == 0 && offline == len(cards) {
		if len(cards) == 1 {
			return "NetLynx: оффлайн — " + nameOf(cards[0])
		}
		return fmt.Sprintf("NetLynx: %d устройств оффлайн", len(cards))
	}
	if online > 0 && offline == 0 && online == len(cards) {
		if len(cards) == 1 {
			return "NetLynx: снова онлайн — " + nameOf(cards[0])
		}
		return fmt.Sprintf("NetLynx: %d устройств снова онлайн", len(cards))
	}
	if len(cards) == 1 {
		return "NetLynx Оповещение — " + nameOf(cards[0])
	}
	return fmt.Sprintf("NetLynx Оповещение (%d)", len(cards))
}

// BuildAlertEmail собирает multipart-ready текст/HTML в стиле UISP (без кнопки «Full Report»).
func BuildAlertEmail(toCSV string, cards []EmailDeviceCard, at time.Time) EmailAlertMail {
	if at.IsZero() {
		at = time.Now()
	}
	if len(cards) == 0 {
		return EmailAlertMail{
			Subject:  "Оповещение NetLynx",
			TextBody: "Оповещение NetLynx\n",
			HTMLBody: "<p>Оповещение NetLynx</p>",
		}
	}
	greet := greetingFromTo(toCSV)

	// Plain-text: приветствие + карточки (без дублирующего intro).
	var text strings.Builder
	text.WriteString("Оповещение NetLynx\n\n")
	text.WriteString(greet + ",\n\n")
	for _, c := range cards {
		text.WriteString(c.Title + "\n")
		if c.Subtitle != "" && c.Subtitle != c.Title {
			text.WriteString(c.Subtitle + "\n")
		}
		if c.Detail != "" && c.Detail != c.Title && c.Detail != c.Subtitle {
			text.WriteString(c.Detail + "\n")
		}
		if c.Attachment != "" {
			text.WriteString("Закреплено " + c.Attachment + "\n")
		}
		text.WriteString(c.StatusLine + "\n")
		if c.URL != "" {
			text.WriteString(c.URL + "\n")
		}
		text.WriteString("\n")
	}

	var cardsHTML bytes.Buffer
	for i, c := range cards {
		accent := "#22a06b"
		if strings.EqualFold(c.Kind, "DEVICE_OFFLINE") {
			accent = "#e5484d"
		} else if !strings.EqualFold(c.Kind, "DEVICE_ONLINE") {
			accent = "#3b82f6"
		}
		borderTop := ""
		if i > 0 {
			borderTop = "border-top:1px solid #e8eaed;"
		}
		title := html.EscapeString(c.Title)
		sub := html.EscapeString(c.Subtitle)
		detail := html.EscapeString(c.Detail)
		att := html.EscapeString(c.Attachment)
		status := html.EscapeString(c.StatusLine)
		href := html.EscapeString(c.URL)
		extra := ""
		if sub != "" && sub != title {
			extra += fmt.Sprintf(`<div style="font-size:14px;color:#64748b;margin-top:2px;line-height:1.35;">%s</div>`, sub)
		}
		if detail != "" && detail != title && detail != sub {
			extra += fmt.Sprintf(`<div style="font-size:14px;color:#64748b;margin-top:2px;line-height:1.35;">%s</div>`, detail)
		}
		if att != "" {
			extra += fmt.Sprintf(`<div style="font-size:14px;color:#64748b;margin-top:2px;line-height:1.35;">Закреплено %s</div>`, att)
		}
		iconCID := html.EscapeString(DeviceIconCID(c.Category))
		// Цветная полоска + иконка типа (white layer) + текст + ›.
		inner := fmt.Sprintf(`
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="%s">
  <tr>
    <td style="width:4px;background:%s;border-radius:4px 0 0 4px;"></td>
    <td style="width:72px;padding:12px 4px 12px 10px;vertical-align:middle;">
      <img src="cid:%s" alt="" width="64" height="36" style="display:block;border:0;width:64px;height:36px;">
    </td>
    <td style="padding:14px 12px 14px 4px;vertical-align:top;">
      <div style="font-size:16px;font-weight:700;color:#111827;line-height:1.3;">%s</div>
      %s
      <div style="font-size:14px;color:%s;margin-top:4px;line-height:1.35;font-weight:600;">%s</div>
    </td>
    <td style="width:28px;text-align:center;vertical-align:top;color:#c0c4cc;font-size:18px;padding:14px 8px 0 0;">›</td>
  </tr>
</table>`, borderTop, accent, iconCID, title, extra, accent, status)
		if href != "" {
			cardsHTML.WriteString(fmt.Sprintf(`<a href="%s" style="text-decoration:none;color:inherit;display:block;">%s</a>`, href, inner))
		} else {
			cardsHTML.WriteString(inner)
		}
	}

	htmlBody := fmt.Sprintf(`<!DOCTYPE html>
<html lang="ru">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"></head>
<body style="margin:0;padding:0;background:#ffffff;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;color:#111827;">
  <table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="background:#ffffff;">
    <tr><td align="center" style="padding:28px 16px 40px;">
      <table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="max-width:480px;margin:0 auto;">
        <tr><td align="center" style="padding-bottom:16px;">
          <img src="cid:netlynx-logo" alt="NetLynx" width="72" height="72" style="display:block;border:0;border-radius:16px;">
        </td></tr>
        <tr><td align="center" style="padding-bottom:20px;">
          <div style="font-size:26px;font-weight:700;letter-spacing:-0.02em;color:#0f172a;">Оповещение NetLynx</div>
        </td></tr>
        <tr><td style="padding-bottom:18px;font-size:16px;line-height:1.5;color:#334155;">
          %s,
        </td></tr>
        <tr><td style="border:1px solid #e5e7eb;border-radius:10px;overflow:hidden;background:#ffffff;">
          %s
        </td></tr>
      </table>
    </td></tr>
  </table>
</body>
</html>`, html.EscapeString(greet), cardsHTML.String())

	return EmailAlertMail{
		Subject:  subjectForCards(cards),
		TextBody: text.String(),
		HTMLBody: htmlBody,
	}
}
