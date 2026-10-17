package snmp

import (
	"strings"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/models"
	"github.com/gosnmp/gosnmp"
)

// UPS-MIB (RFC 1628, 1.3.6.1.2.1.33) — общий стандарт сетевых карт ИБП:
// APC/Schneider, Ippon (EPPC), Eaton, Powercom, CyberPower, Winpower-совместимые NMC.
// Скаляры читаются одним GET-батчем: полный walk группы 33 на Ippon RT II 6000 ≈ 18 с.
const (
	oidUPSSysName = "1.3.6.1.2.1.1.5.0"

	oidUPSIdentManufacturer  = "1.3.6.1.2.1.33.1.1.1.0"
	oidUPSIdentModel         = "1.3.6.1.2.1.33.1.1.2.0"
	oidUPSIdentAgentSoftware = "1.3.6.1.2.1.33.1.1.4.0"
	oidUPSIdentName          = "1.3.6.1.2.1.33.1.1.5.0"

	oidUPSBatteryStatus    = "1.3.6.1.2.1.33.1.2.1.0"
	oidUPSSecondsOnBattery = "1.3.6.1.2.1.33.1.2.2.0"
	oidUPSRuntimeMin       = "1.3.6.1.2.1.33.1.2.3.0"
	oidUPSBatteryChargePct = "1.3.6.1.2.1.33.1.2.4.0"
	oidUPSBatteryVoltage   = "1.3.6.1.2.1.33.1.2.5.0"
	oidUPSBatteryTemp      = "1.3.6.1.2.1.33.1.2.7.0"

	oidUPSInputNumLines  = "1.3.6.1.2.1.33.1.3.2.0"
	oidUPSInputFrequency = "1.3.6.1.2.1.33.1.3.3.1.2.1"
	oidUPSInputVoltage   = "1.3.6.1.2.1.33.1.3.3.1.3.1"
	oidUPSInputCurrent   = "1.3.6.1.2.1.33.1.3.3.1.4.1"

	oidUPSOutputSource    = "1.3.6.1.2.1.33.1.4.1.0"
	oidUPSOutputFrequency = "1.3.6.1.2.1.33.1.4.2.0"
	oidUPSOutputVoltage   = "1.3.6.1.2.1.33.1.4.4.1.2.1"
	oidUPSOutputCurrent   = "1.3.6.1.2.1.33.1.4.4.1.3.1"
	oidUPSOutputPower     = "1.3.6.1.2.1.33.1.4.4.1.4.1"
	oidUPSOutputLoadPct   = "1.3.6.1.2.1.33.1.4.4.1.5.1"

	oidUPSAlarmsPresent        = "1.3.6.1.2.1.33.1.6.1.0"
	oidUPSConfigInputVoltage   = "1.3.6.1.2.1.33.1.9.1.0"
	oidUPSConfigInputFrequency = "1.3.6.1.2.1.33.1.9.2.0"
	oidUPSLowBatteryRuntime    = "1.3.6.1.2.1.33.1.9.7.0"

	// upsOutputSource
	upsSourceNormal  = 3
	upsSourceBattery = 5
)

// upsScalarOIDs — один GET-батч на опрос.
var upsScalarOIDs = []string{
	oidUPSSysName,
	oidUPSIdentManufacturer, oidUPSIdentModel, oidUPSIdentAgentSoftware, oidUPSIdentName,
	oidUPSBatteryStatus, oidUPSSecondsOnBattery, oidUPSRuntimeMin, oidUPSBatteryChargePct,
	oidUPSBatteryVoltage, oidUPSBatteryTemp,
	oidUPSInputNumLines, oidUPSInputFrequency, oidUPSInputVoltage, oidUPSInputCurrent,
	oidUPSOutputSource, oidUPSOutputFrequency, oidUPSOutputVoltage, oidUPSOutputCurrent,
	oidUPSOutputPower, oidUPSOutputLoadPct,
	oidUPSAlarmsPresent, oidUPSConfigInputVoltage, oidUPSConfigInputFrequency, oidUPSLowBatteryRuntime,
}

// UPSMetric — одна точка истории (metric_samples).
type UPSMetric struct {
	Type  string
	Value float32
}

// LooksLikeUPS — ИБП по категории узла или sysDescr карты.
func LooksLikeUPS(category, sysDescr string) bool {
	c := strings.ToLower(strings.TrimSpace(category))
	switch c {
	case "ups", "ибп", "ups_ibp", "power", "питание":
		return true
	}
	d := strings.ToLower(sysDescr)
	needles := []string{
		"ups", "smart-ups", "back-ups", "network management card",
		"winpower", "ippon", "eaton", "powercom", "cyberpower", "santak",
	}
	for _, n := range needles {
		if strings.Contains(d, n) {
			return true
		}
	}
	return false
}

// ReadUPS читает скаляры UPS-MIB одним запросом.
// nil, nil — ответ есть, но ни одного поля UPS-MIB (значит это не ИБП).
func ReadUPS(g *gosnmp.GoSNMP, sysName string) (*models.UPSSnapshot, error) {
	if g == nil {
		return nil, nil
	}
	pdus, err := upsGet(g, upsScalarOIDs)
	if err != nil {
		return nil, err
	}
	u := upsFromPDUs(pdus, time.Now())
	if u == nil {
		return nil, nil
	}
	if u.IdentName == "" {
		u.IdentName = strings.TrimSpace(sysName)
	}
	return u, nil
}

func upsGet(g *gosnmp.GoSNMP, oids []string) (map[string]gosnmp.SnmpPDU, error) {
	pkt, err := g.Get(oids)
	if err != nil {
		return nil, err
	}
	out := make(map[string]gosnmp.SnmpPDU, len(oids))
	if pkt == nil {
		return out, nil
	}
	for _, p := range pkt.Variables {
		out[normalizeOID(p.Name)] = p
	}
	return out, nil
}

// upsFromPDUs — декодирование ответа; чистая функция (тесты без SNMP).
func upsFromPDUs(pdus map[string]gosnmp.SnmpPDU, at time.Time) *models.UPSSnapshot {
	u := &models.UPSSnapshot{PolledAt: at}
	u.Manufacturer = strings.TrimSpace(upsStr(pdus[normalizeOID(oidUPSIdentManufacturer)]))
	u.Model = strings.TrimSpace(upsStr(pdus[normalizeOID(oidUPSIdentModel)]))
	u.IdentName = strings.TrimSpace(upsStr(pdus[normalizeOID(oidUPSIdentName)]))
	u.AgentSoftware = strings.TrimSpace(upsStr(pdus[normalizeOID(oidUPSIdentAgentSoftware)]))

	if v, ok := upsIntOf(pdus[normalizeOID(oidUPSBatteryStatus)], 0); ok {
		u.BatteryStatus = int(v)
	}
	u.SecondsOnBattery = upsIntPtr(pdus[normalizeOID(oidUPSSecondsOnBattery)], 0)
	u.RuntimeMin = upsIntPtr(pdus[normalizeOID(oidUPSRuntimeMin)], 0)
	u.BatteryChargePct = upsClampPct(upsTenthishPtr(pdus[normalizeOID(oidUPSBatteryChargePct)], 1))
	u.BatteryVoltageV = upsTenthishPtr(pdus[normalizeOID(oidUPSBatteryVoltage)], 10)
	u.BatteryTempC = upsTenthishPtr(pdus[normalizeOID(oidUPSBatteryTemp)], 1)

	u.InputLines = upsIntPtr(pdus[normalizeOID(oidUPSInputNumLines)], 0)
	u.InputVoltageV = upsTenthishPtr(pdus[normalizeOID(oidUPSInputVoltage)], 1)
	u.InputFrequencyHz = upsTenthishPtr(pdus[normalizeOID(oidUPSInputFrequency)], 10)
	u.InputCurrentA = upsTenthishPtr(pdus[normalizeOID(oidUPSInputCurrent)], 10)

	if v, ok := upsIntOf(pdus[normalizeOID(oidUPSOutputSource)], 0); ok {
		u.OutputSource = int(v)
	}
	u.OutputVoltageV = upsTenthishPtr(pdus[normalizeOID(oidUPSOutputVoltage)], 1)
	u.OutputCurrentA = upsTenthishPtr(pdus[normalizeOID(oidUPSOutputCurrent)], 10)
	u.OutputPowerW = upsTenthishPtr(pdus[normalizeOID(oidUPSOutputPower)], 1)
	u.OutputLoadPct = upsClampPct(upsTenthishPtr(pdus[normalizeOID(oidUPSOutputLoadPct)], 1))

	u.AlarmsPresent = upsIntPtr(pdus[normalizeOID(oidUPSAlarmsPresent)], 0)
	u.ConfigInputVoltageV = upsTenthishPtr(pdus[normalizeOID(oidUPSConfigInputVoltage)], 1)
	u.LowBatteryRuntimeMin = upsIntPtr(pdus[normalizeOID(oidUPSLowBatteryRuntime)], 0)

	u.OnBattery = u.OutputSource == upsSourceBattery ||
		(u.SecondsOnBattery != nil && *u.SecondsOnBattery > 0)

	if !u.HasData() {
		return nil
	}
	return u
}

// UPSMetricTypes — типы metric_samples для истории ИБП.
var UPSMetricTypes = []string{
	"ups_battery_charge_pct",
	"ups_runtime_min",
	"ups_battery_voltage_v",
	"ups_battery_temp_c",
	"ups_input_voltage_v",
	"ups_output_voltage_v",
	"ups_output_load_pct",
	"ups_output_w",
	"ups_on_battery",
}

// UPSMetricValues — точки истории из снимка (nil-поля пропускаются).
func UPSMetricValues(u *models.UPSSnapshot) []UPSMetric {
	if u == nil {
		return nil
	}
	var out []UPSMetric
	add := func(t string, v *float32) {
		if v != nil {
			out = append(out, UPSMetric{Type: t, Value: *v})
		}
	}
	add("ups_battery_charge_pct", u.BatteryChargePct)
	if u.RuntimeMin != nil {
		out = append(out, UPSMetric{Type: "ups_runtime_min", Value: float32(*u.RuntimeMin)})
	}
	add("ups_battery_voltage_v", u.BatteryVoltageV)
	add("ups_battery_temp_c", u.BatteryTempC)
	add("ups_input_voltage_v", u.InputVoltageV)
	add("ups_output_voltage_v", u.OutputVoltageV)
	add("ups_output_load_pct", u.OutputLoadPct)
	add("ups_output_w", u.OutputPowerW)
	onBattery := float32(0)
	if u.OnBattery {
		onBattery = 1
	}
	out = append(out, UPSMetric{Type: "ups_on_battery", Value: onBattery})
	return out
}

func upsStr(p gosnmp.SnmpPDU) string {
	switch p.Type {
	case gosnmp.NoSuchObject, gosnmp.NoSuchInstance, gosnmp.Null:
		return ""
	}
	// Карты ИБП (EPPC/APC NMC) отдают строки как []byte — берём общий санитайзер.
	return strings.TrimSpace(strings.Trim(SanitizeSNMPValue(p.Value), "\x00"))
}

// upsIntOf — целое из PDU; отрицательные (нет данных у вендора) → ok=false.
func upsIntOf(p gosnmp.SnmpPDU, minValid int64) (int64, bool) {
	if p.Type == gosnmp.NoSuchObject || p.Type == gosnmp.NoSuchInstance || p.Type == gosnmp.Null {
		return 0, false
	}
	switch p.Type {
	case gosnmp.Integer, gosnmp.Counter32, gosnmp.Gauge32, gosnmp.Counter64,
		gosnmp.Uinteger32, gosnmp.TimeTicks, gosnmp.Opaque:
	default:
		return 0, false
	}
	v := pduInt64(p)
	if v < minValid {
		return 0, false
	}
	return v, true
}

func upsIntPtr(p gosnmp.SnmpPDU, minValid int64) *int {
	if v, ok := upsIntOf(p, minValid); ok {
		x := int(v)
		return &x
	}
	return nil
}

// upsTenthishPtr — целое с делителем (0.1 В/А/Гц у UPS-MIB); -1 = не поддержано.
func upsTenthishPtr(p gosnmp.SnmpPDU, div float32) *float32 {
	v, ok := upsIntOf(p, 0)
	if !ok {
		return nil
	}
	x := float32(v) / div
	return &x
}

// upsClampPct — проценты в границах 0…100 (некоторые карты отдают 255).
func upsClampPct(v *float32) *float32 {
	if v == nil {
		return nil
	}
	x := *v
	if x < 0 {
		x = 0
	}
	if x > 100 {
		x = 100
	}
	return &x
}
