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

	// Пороги заряда АКБ (%), с гистерезисом на восстановление.
	upsChargeLowPct      = float32(50)
	upsChargeCriticalPct = float32(20)
	upsChargeOkPct       = float32(60)
	// Падение заряда между опросами без работы от батареи — похоже на проблему с АКБ.
	upsChargeDropPct = float32(10)

	// Быстрый разряд: %/мин (окно не меньше минуты) и минимальное падение за опрос.
	upsDischargeFastPctPerMin = float32(2)
	upsDischargeMinDropPct    = float32(3)
	// не повторять предупреждение о быстром разряде чаще, чем раз в 10 минут.
	upsDischargeCooldown = 10 * time.Minute

	// Автономия на грани и «долго на батарее».
	upsRuntimeLowMin    = 10
	upsOnBatteryLongSec = 600
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
		if a.EventType == "UPS_DISCHARGE_FAST" && !e.upsDischargeAlertDue(d.ID, now) {
			continue
		}
		e.emit(ctx, nil, d, nil, a.EventType, a.Severity, a.Payload)
	}

	if err := e.st.UpdateDeviceUPSSnapshot(ctx, d.ID, cur); err != nil {
		e.log.Warn("ups snapshot", "device_id", d.ID, "err", err)
	}
	e.recordUPSMetrics(ctx, d.ID, prevSnap, cur, now)
}

// upsDischargeAlertDue — антидребезг для предупреждений о быстром разряде
// (разряд продолжается — событие иначе повторялось бы каждый опрос).
func (e *Engine) upsDischargeAlertDue(deviceID int64, now time.Time) bool {
	if v, ok := e.upsDischargeAlertAt.Load(deviceID); ok {
		if last, okT := v.(time.Time); okT && now.Sub(last) < upsDischargeCooldown {
			return false
		}
	}
	e.upsDischargeAlertAt.Store(deviceID, now)
	return true
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

	// Заряд АКБ: пороги (с гистерезисом) + падение без работы от батареи.
	if cur.BatteryChargePct != nil {
		cc := *cur.BatteryChargePct
		var pc float32
		havePC := prev.BatteryChargePct != nil
		if havePC {
			pc = *prev.BatteryChargePct
		}
		switch {
		case cc <= upsChargeCriticalPct && (!havePC || pc > upsChargeCriticalPct):
			add("UPS_CHARGE_CRITICAL", "error")
		case cc <= upsChargeLowPct && (!havePC || pc > upsChargeLowPct):
			add("UPS_CHARGE_LOW", "warning")
		}
		if havePC && cc >= upsChargeOkPct && pc < upsChargeOkPct {
			add("UPS_CHARGE_OK", "info")
		}
		if havePC && !curOn && !prevOn && pc-cc >= upsChargeDropPct {
			add("UPS_CHARGE_DROP", "warning")
		}
	}

	// Быстрый разряд: %/мин по окну между опросами.
	if rate, drop, ok := upsDischargeRate(prev, cur); ok &&
		rate >= upsDischargeFastPctPerMin && drop >= upsDischargeMinDropPct {
		pl["discharge_pct_per_min"] = rate
		pl["charge_drop_pct"] = drop
		add("UPS_DISCHARGE_FAST", "warning")
	}

	// Автономия на грани (работа от батареи).
	if curOn && cur.RuntimeMin != nil && *cur.RuntimeMin <= upsRuntimeLowMin {
		if prev.RuntimeMin == nil || *prev.RuntimeMin > upsRuntimeLowMin {
			add("UPS_RUNTIME_LOW", "warning")
		}
	}

	// Питание не вернулось: на батарее дольше upsOnBatteryLongSec.
	if curOn && cur.SecondsOnBattery != nil {
		csec := *cur.SecondsOnBattery
		psec := 0
		if prev.SecondsOnBattery != nil {
			psec = *prev.SecondsOnBattery
		}
		if csec >= upsOnBatteryLongSec && psec < upsOnBatteryLongSec {
			add("UPS_ON_BATTERY_LONG", "error")
		}
	}
	return out
}

// upsDischargeRate — скорость разряда (%/мин) и падение заряда между опросами.
// Окно меньше минуты не считаем: разрешение метрики — 1%.
func upsDischargeRate(prev, cur *models.UPSSnapshot) (rate, drop float32, ok bool) {
	if prev == nil || cur == nil || !cur.UPSOnBattery() {
		return 0, 0, false
	}
	if prev.BatteryChargePct == nil || cur.BatteryChargePct == nil {
		return 0, 0, false
	}
	dt := cur.PolledAt.Sub(prev.PolledAt)
	if dt < time.Minute {
		return 0, 0, false
	}
	drop = *prev.BatteryChargePct - *cur.BatteryChargePct
	if drop <= 0 {
		return 0, 0, false
	}
	return drop / float32(dt.Minutes()), drop, true
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
