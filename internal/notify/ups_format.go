package notify

import (
	"fmt"
	"strings"
)

// UPS-события: один формат деталей на Telegram и email.
var upsEventTypes = map[string]struct{}{
	"UPS_ON_BATTERY":       {},
	"UPS_MAINS_OK":         {},
	"UPS_BATTERY_LOW":      {},
	"UPS_BATTERY_DEPLETED": {},
	"UPS_BATTERY_OK":       {},
	"UPS_TEMP_HIGH":        {},
	"UPS_TEMP_CRITICAL":    {},
	"UPS_ALARM":            {},
	"UPS_ALARM_CLEAR":      {},
	"UPS_LOAD_HIGH":        {},
	"UPS_LOAD_OK":          {},
}

func isUPSEvent(eventType string) bool {
	_, ok := upsEventTypes[strings.ToUpper(strings.TrimSpace(eventType))]
	return ok
}

// upsEventTitle — заголовок события без подчёркиваний: "UPS ON BATTERY".
func upsEventTitle(eventType string) string {
	return strings.ReplaceAll(strings.ToUpper(strings.TrimSpace(eventType)), "_", " ")
}

// upsPayloadDetail — «модель · заряд 97% · 20 мин · батарея 218.7 В · нагрузка 95%».
func upsPayloadDetail(payload map[string]interface{}) string {
	if payload == nil {
		return "?"
	}
	var parts []string
	if model, _ := payload["ups_model"].(string); strings.TrimSpace(model) != "" {
		parts = append(parts, strings.TrimSpace(model))
	}
	if _, ok := payload["battery_charge_pct"]; ok {
		parts = append(parts, fmt.Sprintf("заряд %.0f%%", payloadFloat(payload, "battery_charge_pct")))
	}
	if _, ok := payload["runtime_min"]; ok {
		parts = append(parts, fmt.Sprintf("%.0f мин", payloadFloat(payload, "runtime_min")))
	}
	if _, ok := payload["battery_voltage_v"]; ok {
		parts = append(parts, fmt.Sprintf("АКБ %.1f В", payloadFloat(payload, "battery_voltage_v")))
	}
	if _, ok := payload["battery_temp_c"]; ok {
		parts = append(parts, fmt.Sprintf("%.0f °C", payloadFloat(payload, "battery_temp_c")))
	}
	if _, ok := payload["input_voltage_v"]; ok {
		parts = append(parts, fmt.Sprintf("вход %.0f В", payloadFloat(payload, "input_voltage_v")))
	}
	if _, ok := payload["output_voltage_v"]; ok {
		parts = append(parts, fmt.Sprintf("выход %.0f В", payloadFloat(payload, "output_voltage_v")))
	}
	if _, ok := payload["output_load_pct"]; ok {
		parts = append(parts, fmt.Sprintf("нагрузка %.0f%%", payloadFloat(payload, "output_load_pct")))
	}
	if _, ok := payload["output_w"]; ok {
		parts = append(parts, fmt.Sprintf("%.0f Вт", payloadFloat(payload, "output_w")))
	}
	if _, ok := payload["alarms_present"]; ok {
		parts = append(parts, fmt.Sprintf("аварий %.0f", payloadFloat(payload, "alarms_present")))
	}
	if len(parts) == 0 {
		return "?"
	}
	return strings.Join(parts, " · ")
}
