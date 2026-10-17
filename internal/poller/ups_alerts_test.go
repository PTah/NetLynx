package poller

import (
	"testing"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/models"
)

func f32(v float32) *float32 { return &v }
func iptr(v int) *int        { return &v }

func upsBase() *models.UPSSnapshot {
	return &models.UPSSnapshot{
		Manufacturer:     "EPPC",
		Model:            "ON-LINE",
		IdentName:        "Ippon RT II 6000",
		BatteryStatus:    2,
		BatteryChargePct: f32(100),
		BatteryVoltageV:  f32(218.7),
		BatteryTempC:     f32(25),
		RuntimeMin:       iptr(38),
		OutputSource:     3,
		OutputVoltageV:   f32(220),
		OutputLoadPct:    f32(20),
		AlarmsPresent:    iptr(0),
		PolledAt:         time.Now().Add(-time.Minute),
	}
}

func alertTypes(alerts []upsAlert) []string {
	out := make([]string, 0, len(alerts))
	for _, a := range alerts {
		out = append(out, a.EventType)
	}
	return out
}

func hasAlert(alerts []upsAlert, typ string) bool {
	for _, a := range alerts {
		if a.EventType == typ {
			return true
		}
	}
	return false
}

func TestUPSAlertsFirstSnapshotSilent(t *testing.T) {
	if got := upsAlertEvents(nil, upsBase()); got != nil {
		t.Fatalf("первый снимок не должен давать алертов: %v", alertTypes(got))
	}
	// prev без данных (свитч, который раньше отвечал только IF-MIB) — тоже тишина.
	if got := upsAlertEvents(&models.UPSSnapshot{}, upsBase()); got != nil {
		t.Fatalf("пустой prev не должен давать алертов: %v", alertTypes(got))
	}
}

func TestUPSAlertsOnBatteryAndMainsBack(t *testing.T) {
	prev := upsBase()
	onBatt := upsBase()
	onBatt.OutputSource = 5
	onBatt.SecondsOnBattery = iptr(30)
	onBatt.BatteryChargePct = f32(97)
	onBatt.RuntimeMin = iptr(20)

	alerts := upsAlertEvents(prev, onBatt)
	if !hasAlert(alerts, "UPS_ON_BATTERY") {
		t.Fatalf("ожидался UPS_ON_BATTERY: %v", alertTypes(alerts))
	}
	for _, a := range alerts {
		if a.EventType == "UPS_ON_BATTERY" && a.Severity != "error" {
			t.Fatalf("severity UPS_ON_BATTERY: %s", a.Severity)
		}
	}
	pl := alerts[0].Payload
	if pl["ups_model"] != "Ippon RT II 6000" || pl["runtime_min"] != 20 || pl["on_battery"] != true {
		t.Fatalf("payload: %v", pl)
	}

	back := upsAlertEvents(onBatt, prev)
	if !hasAlert(back, "UPS_MAINS_OK") || hasAlert(back, "UPS_ON_BATTERY") {
		t.Fatalf("ожидался UPS_MAINS_OK: %v", alertTypes(back))
	}
}

func TestUPSAlertsBatteryStatus(t *testing.T) {
	prev := upsBase()
	low := upsBase()
	low.BatteryStatus = upsBatteryLow
	if got := upsAlertEvents(prev, low); !hasAlert(got, "UPS_BATTERY_LOW") {
		t.Fatalf("UPS_BATTERY_LOW: %v", alertTypes(got))
	}
	// Повторный опрос с тем же статусом — без повторов.
	if got := upsAlertEvents(low, low); len(got) != 0 {
		t.Fatalf("без изменений алертов быть не должно: %v", alertTypes(got))
	}
	depleted := upsBase()
	depleted.BatteryStatus = upsBatteryDeplet
	if got := upsAlertEvents(low, depleted); !hasAlert(got, "UPS_BATTERY_DEPLETED") {
		t.Fatalf("UPS_BATTERY_DEPLETED: %v", alertTypes(got))
	}
	if got := upsAlertEvents(depleted, prev); !hasAlert(got, "UPS_BATTERY_OK") {
		t.Fatalf("UPS_BATTERY_OK: %v", alertTypes(got))
	}
}

func TestUPSAlertsTempAlarmsLoad(t *testing.T) {
	prev := upsBase()

	hot := upsBase()
	hot.BatteryTempC = f32(47)
	if got := upsAlertEvents(prev, hot); !hasAlert(got, "UPS_TEMP_HIGH") {
		t.Fatalf("UPS_TEMP_HIGH: %v", alertTypes(got))
	}
	hotter := upsBase()
	hotter.BatteryTempC = f32(57)
	if got := upsAlertEvents(hot, hotter); !hasAlert(got, "UPS_TEMP_CRITICAL") {
		t.Fatalf("UPS_TEMP_CRITICAL: %v", alertTypes(got))
	}

	alarm := upsBase()
	alarm.AlarmsPresent = iptr(2)
	if got := upsAlertEvents(prev, alarm); !hasAlert(got, "UPS_ALARM") {
		t.Fatalf("UPS_ALARM: %v", alertTypes(got))
	}
	if got := upsAlertEvents(alarm, prev); !hasAlert(got, "UPS_ALARM_CLEAR") {
		t.Fatalf("UPS_ALARM_CLEAR: %v", alertTypes(got))
	}

	loaded := upsBase()
	loaded.OutputLoadPct = f32(95)
	if got := upsAlertEvents(prev, loaded); !hasAlert(got, "UPS_LOAD_HIGH") {
		t.Fatalf("UPS_LOAD_HIGH: %v", alertTypes(got))
	}
	relaxed := upsBase()
	relaxed.OutputLoadPct = f32(50)
	if got := upsAlertEvents(loaded, relaxed); !hasAlert(got, "UPS_LOAD_OK") {
		t.Fatalf("UPS_LOAD_OK: %v", alertTypes(got))
	}
}

func TestUPSMetricDue(t *testing.T) {
	now := time.Now()
	if !upsMetricDue(nil, now) {
		t.Fatal("без предыдущего снимка семпл нужен")
	}
	recent := &models.UPSSnapshot{PolledAt: now.Add(-time.Minute)}
	if upsMetricDue(recent, now) {
		t.Fatal("минуту назад — рано")
	}
	old := &models.UPSSnapshot{PolledAt: now.Add(-upsMetricInterval - time.Second)}
	if !upsMetricDue(old, now) {
		t.Fatal("интервал прошёл — нужен семпл")
	}
}

func TestUPSAlertsChargeThresholds(t *testing.T) {
	full := upsBase() // 100%
	full.PolledAt = time.Now().Add(-2 * time.Minute)

	low := upsBase()
	low.BatteryChargePct = f32(45)
	low.PolledAt = time.Now().Add(-time.Minute)
	if got := upsAlertEvents(full, low); !hasAlert(got, "UPS_CHARGE_LOW") {
		t.Fatalf("UPS_CHARGE_LOW: %v", alertTypes(got))
	}
	// Внутри того же порога повторов быть не должно.
	lower := upsBase()
	lower.BatteryChargePct = f32(35)
	if got := upsAlertEvents(low, lower); hasAlert(got, "UPS_CHARGE_LOW") {
		t.Fatalf("повтор UPS_CHARGE_LOW: %v", alertTypes(got))
	}

	crit := upsBase()
	crit.BatteryChargePct = f32(15)
	if got := upsAlertEvents(lower, crit); !hasAlert(got, "UPS_CHARGE_CRITICAL") {
		t.Fatalf("UPS_CHARGE_CRITICAL: %v", alertTypes(got))
	}

	back := upsBase()
	back.BatteryChargePct = f32(75)
	if got := upsAlertEvents(crit, back); !hasAlert(got, "UPS_CHARGE_OK") {
		t.Fatalf("UPS_CHARGE_OK: %v", alertTypes(got))
	}
}

func TestUPSAlertsChargeDropWithoutBattery(t *testing.T) {
	prev := upsBase() // не на батарее, 100%
	cur := upsBase()
	cur.BatteryChargePct = f32(85) // падение на 15% на сети — похоже на проблему АКБ
	if got := upsAlertEvents(prev, cur); !hasAlert(got, "UPS_CHARGE_DROP") {
		t.Fatalf("UPS_CHARGE_DROP: %v", alertTypes(got))
	}
	// Небольшое падение (5%) — тишина.
	small := upsBase()
	small.BatteryChargePct = f32(95)
	if got := upsAlertEvents(prev, small); hasAlert(got, "UPS_CHARGE_DROP") {
		t.Fatalf("лишний UPS_CHARGE_DROP: %v", alertTypes(got))
	}
}

func TestUPSAlertsDischargeFast(t *testing.T) {
	now := time.Now()
	prev := upsBase()
	prev.BatteryChargePct = f32(100)
	prev.PolledAt = now.Add(-2 * time.Minute)
	prev.OnBattery = true
	prev.SecondsOnBattery = iptr(120)

	cur := upsBase()
	cur.BatteryChargePct = f32(90) // -10% за 2 минуты = 5%/мин
	cur.PolledAt = now
	cur.OnBattery = true
	cur.SecondsOnBattery = iptr(240)

	rate, drop, ok := upsDischargeRate(prev, cur)
	if !ok || drop != 10 || rate < 4.9 || rate > 5.1 {
		t.Fatalf("rate=%v drop=%v ok=%v", rate, drop, ok)
	}
	alerts := upsAlertEvents(prev, cur)
	if !hasAlert(alerts, "UPS_DISCHARGE_FAST") {
		t.Fatalf("UPS_DISCHARGE_FAST: %v", alertTypes(alerts))
	}
	for _, a := range alerts {
		if a.EventType == "UPS_DISCHARGE_FAST" && a.Payload["discharge_pct_per_min"] == nil {
			t.Fatal("в payload нет скорости разряда")
		}
	}
	// Окно меньше минуты — не считаем.
	short := upsBase()
	short.BatteryChargePct = f32(95)
	short.PolledAt = now.Add(-30 * time.Second)
	short.OnBattery = true
	if _, _, ok := upsDischargeRate(short, cur); ok {
		t.Fatal("окно < 1 мин не должно считаться")
	}
	// На сети быстрый разряд не считаем.
	mains := upsBase()
	mains.BatteryChargePct = f32(95)
	mains.PolledAt = now.Add(-2 * time.Minute)
	mains.OnBattery = false
	mains.SecondsOnBattery = iptr(0)
	mains.OutputSource = 3
	curMains := upsBase()
	curMains.BatteryChargePct = f32(80)
	curMains.OutputSource = 3
	curMains.SecondsOnBattery = iptr(0)
	if _, _, ok := upsDischargeRate(mains, curMains); ok {
		t.Fatal("на сети скорость разряда не считаем")
	}
}

func TestUPSAlertsRuntimeLowAndLongOnBattery(t *testing.T) {
	now := time.Now()
	prev := upsBase()
	prev.OnBattery = true
	prev.SecondsOnBattery = iptr(300)
	prev.RuntimeMin = iptr(25)
	prev.PolledAt = now.Add(-time.Minute)

	cur := upsBase()
	cur.OnBattery = true
	cur.SecondsOnBattery = iptr(660) // > 10 минут на батарее
	cur.RuntimeMin = iptr(8)         // автономии мало
	cur.PolledAt = now

	alerts := upsAlertEvents(prev, cur)
	if !hasAlert(alerts, "UPS_RUNTIME_LOW") {
		t.Fatalf("UPS_RUNTIME_LOW: %v", alertTypes(alerts))
	}
	if !hasAlert(alerts, "UPS_ON_BATTERY_LONG") {
		t.Fatalf("UPS_ON_BATTERY_LONG: %v", alertTypes(alerts))
	}
	// Продолжаем сидеть на батарее — повторов нет.
	again := upsBase()
	again.OnBattery = true
	again.SecondsOnBattery = iptr(900)
	again.RuntimeMin = iptr(5)
	if got := upsAlertEvents(cur, again); len(got) != 0 {
		t.Fatalf("повторы не нужны: %v", alertTypes(got))
	}
}
