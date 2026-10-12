package notify

import (
	"strings"
	"testing"
	"time"
)

func TestFormatTelegramFromCardOffline(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Skip(err)
	}
	at := time.Date(2026, 8, 28, 17, 13, 0, 0, loc)
	payload := map[string]interface{}{
		"offline_since":   "2026-08-28T07:11:35Z",
		"device_category": "computer",
	}
	card := EmailDeviceCard{
		Title:      "Operator-3",
		Category:   "computer",
		Attachment: "на коммутаторе «EdgeSwitch 8 #9 (Operatory-2)», порт 0/1",
	}
	got := formatTelegramFromCard(card, "192.168.160.118", "DEVICE_OFFLINE", payload, at)
	offlineClock := telegramClock(payloadEventClock("DEVICE_OFFLINE", payload, at))
	wantParts := []string{
		"<b>NetLynx</b>",
		"<b><u>DEVICE OFFLINE</u></b> с " + offlineClock,
		"Компьютер: Operator-3 (192.168.160.118)",
		"(Закреплено на коммутаторе «EdgeSwitch 8 #9 (Operatory-2)», порт 0/1)",
	}
	for _, p := range wantParts {
		if !strings.Contains(got, p) {
			t.Fatalf("missing %q in:\n%s", p, got)
		}
	}
	if strings.Contains(got, "map[") {
		t.Fatalf("raw payload leak: %s", got)
	}
}

func TestFormatTelegramFromCardOnline(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Skip(err)
	}
	at := time.Date(2026, 8, 28, 8, 53, 12, 0, loc)
	payload := map[string]interface{}{
		"offline_duration_sec": 226260, // 62h 51m
		"device_category":      "computer",
	}
	card := EmailDeviceCard{
		Title:      "COMM-PC",
		Category:   "computer",
		Attachment: "на коммутаторе «EdgeSwitch 8 #9 (Operatory-2)», порт 0/1",
	}
	got := formatTelegramFromCard(card, "192.168.162.33", "DEVICE_ONLINE", payload, at)
	onlineClock := telegramClock(at.Local())
	wantParts := []string{
		"<b>NetLynx</b>",
		"<b><u>DEVICE ONLINE</u></b> с " + onlineClock,
		"(время оффлайна: 62 часа 51 минута)",
		"Компьютер: COMM-PC (192.168.162.33)",
		"(Закреплено на коммутаторе «EdgeSwitch 8 #9 (Operatory-2)», порт 0/1)",
	}
	for _, p := range wantParts {
		if !strings.Contains(got, p) {
			t.Fatalf("missing %q in:\n%s", p, got)
		}
	}
	// Duration must be on its own line, not glued to ONLINE clock.
	if strings.Contains(got, onlineClock+" (время") {
		t.Fatalf("duration should be on separate line:\n%s", got)
	}
}

func TestFormatTelegramDurationRU(t *testing.T) {
	if got := formatTelegramDurationRU(45000); got != "12 часов 30 минут" {
		t.Fatalf("got %q", got)
	}
	if got := formatTelegramDurationRU(45); got != "менее минуты" {
		t.Fatalf("got %q", got)
	}
	if got := formatTelegramDurationRU(226260); got != "62 часа 51 минута" {
		t.Fatalf("got %q", got)
	}
}

func TestEscapeTelegramHTML(t *testing.T) {
	if got := escapeTelegramHTML("a & b <c>"); got != "a &amp; b &lt;c&gt;" {
		t.Fatalf("got %q", got)
	}
}
