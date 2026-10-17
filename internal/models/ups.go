package models

import (
	"fmt"
	"strings"
	"time"
)

// UPSSnapshot — снимок UPS-MIB (RFC 1628, 1.3.6.1.2.1.33) на момент опроса.
// Числовые поля, помеченные производителем как «не поддержано» (-1), остаются nil.
type UPSSnapshot struct {
	Manufacturer  string `json:"manufacturer,omitempty"`
	Model         string `json:"model,omitempty"`
	IdentName     string `json:"ident_name,omitempty"`
	AgentSoftware string `json:"agent_software,omitempty"`

	/** 1 unknown, 2 normal, 3 low, 4 depleted */
	BatteryStatus    int      `json:"battery_status"`
	SecondsOnBattery *int     `json:"seconds_on_battery,omitempty"`
	RuntimeMin       *int     `json:"runtime_min,omitempty"`
	BatteryChargePct *float32 `json:"battery_charge_pct,omitempty"`
	BatteryVoltageV  *float32 `json:"battery_voltage_v,omitempty"`
	BatteryTempC     *float32 `json:"battery_temp_c,omitempty"`
	/** Питание пропало: secondsOnBattery > 0 или outputSource = battery. */
	OnBattery bool `json:"on_battery"`

	InputLines       *int     `json:"input_lines,omitempty"`
	InputVoltageV    *float32 `json:"input_voltage_v,omitempty"`
	InputFrequencyHz *float32 `json:"input_frequency_hz,omitempty"`
	InputCurrentA    *float32 `json:"input_current_a,omitempty"`

	/** 1 other, 2 none, 3 normal, 4 bypass, 5 battery, 6 booster, 7 reducer */
	OutputSource   int      `json:"output_source"`
	OutputVoltageV *float32 `json:"output_voltage_v,omitempty"`
	OutputCurrentA *float32 `json:"output_current_a,omitempty"`
	OutputPowerW   *float32 `json:"output_power_w,omitempty"`
	OutputLoadPct  *float32 `json:"output_load_pct,omitempty"`

	AlarmsPresent        *int     `json:"alarms_present,omitempty"`
	ConfigInputVoltageV  *float32 `json:"config_input_voltage_v,omitempty"`
	LowBatteryRuntimeMin *int     `json:"low_battery_runtime_min,omitempty"`

	PolledAt time.Time `json:"polled_at"`
}

// HasData — снимок содержит что-то содержательное (узел действительно ИБП).
func (u *UPSSnapshot) HasData() bool {
	return u != nil && (u.Manufacturer != "" || u.Model != "" || u.IdentName != "" ||
		u.BatteryStatus > 0 || u.BatteryChargePct != nil || u.OutputVoltageV != nil)
}

// UPSBatteryStatusLabel — upsBatteryStatus словами.
func UPSBatteryStatusLabel(v int) string {
	switch v {
	case 2:
		return "норма"
	case 3:
		return "низкий заряд"
	case 4:
		return "разряжена"
	case 1:
		return "неизвестно"
	default:
		return "—"
	}
}

// UPSOutputSourceLabel — upsOutputSource словами.
func UPSOutputSourceLabel(v int) string {
	switch v {
	case 1:
		return "иное"
	case 2:
		return "нет выхода"
	case 3:
		return "сеть"
	case 4:
		return "байпас"
	case 5:
		return "батарея"
	case 6:
		return "booster"
	case 7:
		return "reducer"
	default:
		return "—"
	}
}

// UPSTitle — человекочитаемое имя ИБП: identName, иначе «модель (manufacturer)».
func (u *UPSSnapshot) UPSTitle() string {
	if u == nil {
		return ""
	}
	if s := strings.TrimSpace(u.IdentName); s != "" {
		return s
	}
	model := strings.TrimSpace(u.Model)
	maker := strings.TrimSpace(u.Manufacturer)
	switch {
	case model != "" && maker != "":
		return fmt.Sprintf("%s (%s)", model, maker)
	case model != "":
		return model
	default:
		return maker
	}
}

// UPSOnBattery — работа от батареи по снимку.
func (u *UPSSnapshot) UPSOnBattery() bool {
	if u == nil {
		return false
	}
	if u.SecondsOnBattery != nil && *u.SecondsOnBattery > 0 {
		return true
	}
	return u.OutputSource == 5
}
