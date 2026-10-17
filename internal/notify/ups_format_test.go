package notify

import (
	"strings"
	"testing"
	"time"
)

func TestUPSEventFormatting(t *testing.T) {
	payload := map[string]interface{}{
		"ups_model":          "Ippon RT II 6000",
		"on_battery":         true,
		"battery_status":     3,
		"battery_charge_pct": float32(97),
		"runtime_min":        20,
		"battery_voltage_v":  float32(218.7),
		"battery_temp_c":     float32(47),
		"output_voltage_v":   float32(220),
		"output_load_pct":    float32(95),
		"output_w":           float32(1100),
		"alarms_present":     1,
	}
	if !isUPSEvent("ups_on_battery") {
		t.Fatal("UPS_ON_BATTERY должен распознаваться без учёта регистра")
	}
	if isUPSEvent("TONER_LOW") {
		t.Fatal("TONER_LOW — не UPS-событие")
	}
	if got := upsEventTitle("UPS_ON_BATTERY"); got != "UPS ON BATTERY" {
		t.Fatalf("title: %q", got)
	}
	detail := upsPayloadDetail(payload)
	for _, want := range []string{"Ippon RT II 6000", "заряд 97%", "20 мин", "АКБ 218.7 В", "47 °C", "нагрузка 95%", "аварий 1"} {
		if !strings.Contains(detail, want) {
			t.Fatalf("в деталях нет %q: %s", want, detail)
		}
	}
	tg := telegramEventLine("UPS_ON_BATTERY", payload, time.Now())
	if !strings.Contains(tg, "UPS ON BATTERY") || !strings.Contains(tg, "заряд 97%") {
		t.Fatalf("telegram: %s", tg)
	}
	card := cardStatusLine("UPS_BATTERY_LOW", "ups", payload, time.Now())
	if !strings.Contains(card, "ИБП") || !strings.Contains(card, "заряд 97%") {
		t.Fatalf("email-карточка: %s", card)
	}
}
