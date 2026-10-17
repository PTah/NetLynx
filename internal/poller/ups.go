package poller

import (
	"context"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/models"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/snmp"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
	"github.com/gosnmp/gosnmp"
)

const (
	// upsMetricInterval — как часто писать историю ИБП (опрос идёт чаще).
	upsMetricInterval = 5 * time.Minute
	// допуск джиттера расписания опроса, чтобы семпл не «съезжал» на минуту.
	upsMetricJitter = 20 * time.Second

	upsTempHighC     = float32(45)
	upsTempCritC     = float32(55)
	upsLoadHighPct   = float32(90)
	upsLoadOkPct     = float32(80)
	upsBatteryLow    = 3 // upsBatteryStatus: low
	upsBatteryDeplet = 4 // upsBatteryStatus: depleted
	upsBatteryNormal = 2 // upsBatteryStatus: normal
)

// upsAlert — событие по переходу состояния ИБП.
type upsAlert struct {
	EventType string
	Severity  string
	Payload   map[string]interface{}
}

// pollUPS: читает UPS-MIB, сохраняет снимок, пишет историю и события.
func (e *Engine) pollUPS(ctx context.Context, d store.PollDevice, prev *models.Device, g *gosnmp.GoSNMP, sysName string, now time.Time) {
	cur, err := snmp.ReadUPS(g, sysName)
	if err != nil {
		e.log.Debug("ups mib", "device_id", d.ID, "err", err)
		return
	}
	if cur == nil {
		return
	}
	cur.PolledAt = now

	var prevSnap *models.UPSSnapshot
	if prev != nil {
		prevSnap = prev.LastUPS
	}

	for _, a := range upsAlertEvents(prevSnap, cur) {
		e.emit(ctx, nil, d, nil, a.EventType, a.Severity, a.Payload)
	}

	if err := e.st.UpdateDeviceUPSSnapshot(ctx, d.ID, cur); err != nil {
		e.log.Warn("ups snapshot", "device_id", d.ID, "err", err)
	}
	e.recordUPSMetrics(ctx, d.ID, prevSnap, cur, now)
}

// recordUPSMetrics — история ups_* в metric_samples не чаще upsMetricInterval,
// но обязательно при смене питания (батарея ↔ сеть).
func (e *Engine) recordUPSMetrics(ctx context.Context, deviceID int64, prevSnap, cur *models.UPSSnapshot, now time.Time) {
	if !e.cfg.MetricsEnabled {
		return
	}
	stateChanged := prevSnap == nil || prevSnap.UPSOnBattery() != cur.UPSOnBattery()
	if !stateChanged && !upsMetricDue(prevSnap, now) {
		return
	}
	var samples []store.MetricSample
	for _, m := range snmp.UPSMetricValues(cur) {
		samples = append(samples, store.MetricSample{MetricType: m.Type, Value: m.Value, SampledAt: now})
	}
	if len(samples) == 0 {
		return
	}
	if err := e.st.InsertMetricSamplesBatch(ctx, deviceID, samples); err != nil {
		e.log.Warn("ups metric samples", "device_id", deviceID, "err", err)
	}
}

// upsMetricDue — прошёл ли интервал с последнего семпла истории.
func upsMetricDue(prev *models.UPSSnapshot, now time.Time) bool {
	if prev == nil || prev.PolledAt.IsZero() {
		return true
	}
	return now.Sub(prev.PolledAt) >= upsMetricInterval-upsMetricJitter
}

// upsAlertEvents — переходы состояния относительно предыдущего снимка.
// Первый снимок (prev без данных) алертов не даёт: это базовый уровень.
func upsAlertEvents(prev, cur *models.UPSSnapshot) []upsAlert {
	if cur == nil || prev == nil || !prev.HasData() {
		return nil
	}
	pl := upsEventPayload(cur)
	var out []upsAlert
	add := func(t, sev string) {
		out = append(out, upsAlert{EventType: t, Severity: sev, Payload: pl})
	}

	prevOn, curOn := prev.UPSOnBattery(), cur.UPSOnBattery()
	if !prevOn && curOn {
		add("UPS_ON_BATTERY", "error")
	}
	if prevOn && !curOn {
		add("UPS_MAINS_OK", "info")
	}

	switch {
	case cur.BatteryStatus == upsBatteryDeplet && prev.BatteryStatus != upsBatteryDeplet:
		add("UPS_BATTERY_DEPLETED", "error")
	case cur.BatteryStatus == upsBatteryLow && prev.BatteryStatus != upsBatteryLow:
		add("UPS_BATTERY_LOW", "warning")
	}
	if cur.BatteryStatus == upsBatteryNormal &&
		(prev.BatteryStatus == upsBatteryLow || prev.BatteryStatus == upsBatteryDeplet) {
		add("UPS_BATTERY_OK", "info")
	}

	if cur.BatteryTempC != nil && prev.BatteryTempC != nil {
		ct, pt := *cur.BatteryTempC, *prev.BatteryTempC
		switch {
		case ct >= upsTempCritC && pt < upsTempCritC:
			add("UPS_TEMP_CRITICAL", "error")
		case ct >= upsTempHighC && pt < upsTempHighC:
			add("UPS_TEMP_HIGH", "warning")
		}
	}

	prevAlarms := 0
	if prev.AlarmsPresent != nil {
		prevAlarms = *prev.AlarmsPresent
	}
	curAlarms := 0
	if cur.AlarmsPresent != nil {
		curAlarms = *cur.AlarmsPresent
	}
	if prevAlarms == 0 && curAlarms > 0 {
		add("UPS_ALARM", "warning")
	}
	if prevAlarms > 0 && curAlarms == 0 {
		add("UPS_ALARM_CLEAR", "info")
	}

	if cur.OutputLoadPct != nil && prev.OutputLoadPct != nil {
		cl, pl := *cur.OutputLoadPct, *prev.OutputLoadPct
		if cl >= upsLoadHighPct && pl < upsLoadHighPct {
			add("UPS_LOAD_HIGH", "warning")
		}
		if pl >= upsLoadHighPct && cl < upsLoadOkPct {
			add("UPS_LOAD_OK", "info")
		}
	}
	return out
}

// upsEventPayload — общие поля события ИБП (для журнала/уведомлений).
func upsEventPayload(u *models.UPSSnapshot) map[string]interface{} {
	if u == nil {
		return nil
	}
	pl := map[string]interface{}{
		"on_battery":     u.UPSOnBattery(),
		"battery_status": u.BatteryStatus,
		"output_source":  u.OutputSource,
	}
	if t := u.UPSTitle(); t != "" {
		pl["ups_model"] = t
	}
	if u.BatteryChargePct != nil {
		pl["battery_charge_pct"] = *u.BatteryChargePct
	}
	if u.RuntimeMin != nil {
		pl["runtime_min"] = *u.RuntimeMin
	}
	if u.SecondsOnBattery != nil {
		pl["seconds_on_battery"] = *u.SecondsOnBattery
	}
	if u.BatteryVoltageV != nil {
		pl["battery_voltage_v"] = *u.BatteryVoltageV
	}
	if u.BatteryTempC != nil {
		pl["battery_temp_c"] = *u.BatteryTempC
	}
	if u.InputVoltageV != nil {
		pl["input_voltage_v"] = *u.InputVoltageV
	}
	if u.OutputVoltageV != nil {
		pl["output_voltage_v"] = *u.OutputVoltageV
	}
	if u.OutputPowerW != nil {
		pl["output_w"] = *u.OutputPowerW
	}
	if u.OutputLoadPct != nil {
		pl["output_load_pct"] = *u.OutputLoadPct
	}
	if u.AlarmsPresent != nil {
		pl["alarms_present"] = *u.AlarmsPresent
	}
	return pl
}
