package snmp

import (
	"os"
	"strconv"
	"testing"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/models"
	"github.com/gosnmp/gosnmp"
)

func upsPDU(oid string, typ gosnmp.Asn1BER, v interface{}) gosnmp.SnmpPDU {
	return gosnmp.SnmpPDU{Name: "." + oid, Type: typ, Value: v}
}

// upsPDUStr — строковый PDU так, как его отдаёт карта ИБП: []byte (не string).
func upsPDUStr(oid, v string) gosnmp.SnmpPDU {
	return gosnmp.SnmpPDU{Name: "." + oid, Type: gosnmp.OctetString, Value: []byte(v)}
}

// Реальный ответ Ippon RT II 6000 (карта EPPC, sysObjectID 1.3.6.1.4.1.935.10.1).
func ipponPDUs() map[string]gosnmp.SnmpPDU {
	return map[string]gosnmp.SnmpPDU{
		oidUPSSysName:              upsPDUStr(oidUPSSysName, "NMC"),
		oidUPSIdentManufacturer:    upsPDUStr(oidUPSIdentManufacturer, "EPPC"),
		oidUPSIdentModel:           upsPDUStr(oidUPSIdentModel, "ON-LINE"),
		oidUPSIdentName:            upsPDUStr(oidUPSIdentName, "Ippon RT II 6000"),
		oidUPSIdentAgentSoftware:   upsPDUStr(oidUPSIdentAgentSoftware, "3.1.0.0"),
		oidUPSBatteryStatus:        upsPDU(oidUPSBatteryStatus, gosnmp.Integer, 2),
		oidUPSSecondsOnBattery:     upsPDU(oidUPSSecondsOnBattery, gosnmp.Integer, 0),
		oidUPSRuntimeMin:           upsPDU(oidUPSRuntimeMin, gosnmp.Integer, 37),
		oidUPSBatteryChargePct:     upsPDU(oidUPSBatteryChargePct, gosnmp.Integer, 100),
		oidUPSBatteryVoltage:       upsPDU(oidUPSBatteryVoltage, gosnmp.Integer, 2187),
		oidUPSBatteryTemp:          upsPDU(oidUPSBatteryTemp, gosnmp.Integer, 25),
		oidUPSInputNumLines:        upsPDU(oidUPSInputNumLines, gosnmp.Integer, 1),
		oidUPSInputFrequency:       upsPDU(oidUPSInputFrequency, gosnmp.Integer, 499),
		oidUPSInputVoltage:         upsPDU(oidUPSInputVoltage, gosnmp.Integer, 238),
		oidUPSInputCurrent:         upsPDU(oidUPSInputCurrent, gosnmp.Integer, -1),
		oidUPSOutputSource:         upsPDU(oidUPSOutputSource, gosnmp.Integer, 3),
		oidUPSOutputVoltage:        upsPDU(oidUPSOutputVoltage, gosnmp.Integer, 220),
		oidUPSOutputCurrent:        upsPDU(oidUPSOutputCurrent, gosnmp.Integer, 56),
		oidUPSOutputPower:          upsPDU(oidUPSOutputPower, gosnmp.Integer, 1100),
		oidUPSOutputLoadPct:        upsPDU(oidUPSOutputLoadPct, gosnmp.Integer, 20),
		oidUPSAlarmsPresent:        upsPDU(oidUPSAlarmsPresent, gosnmp.Gauge32, 0),
		oidUPSConfigInputVoltage:   upsPDU(oidUPSConfigInputVoltage, gosnmp.Integer, 220),
		oidUPSConfigInputFrequency: upsPDU(oidUPSConfigInputFrequency, gosnmp.Integer, 500),
		oidUPSLowBatteryRuntime:    upsPDU(oidUPSLowBatteryRuntime, gosnmp.Integer, 0),
	}
}

func TestDecodeUPSSnapshot(t *testing.T) {
	at := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	u := upsFromPDUs(ipponPDUs(), at)
	if u == nil {
		t.Fatal("снимок не декодирован")
	}
	if u.IdentName != "Ippon RT II 6000" || u.Manufacturer != "EPPC" || u.Model != "ON-LINE" {
		t.Fatalf("идентификация: %+v", u)
	}
	if u.BatteryStatus != 2 || u.OnBattery {
		t.Fatalf("батарея: status=%d onBattery=%v", u.BatteryStatus, u.OnBattery)
	}
	if u.BatteryChargePct == nil || *u.BatteryChargePct != 100 {
		t.Fatalf("заряд: %v", u.BatteryChargePct)
	}
	if u.BatteryVoltageV == nil || *u.BatteryVoltageV != 218.7 {
		t.Fatalf("напряжение батареи 0.1В: %v", u.BatteryVoltageV)
	}
	if u.InputFrequencyHz == nil || *u.InputFrequencyHz != 49.9 {
		t.Fatalf("частота входа 0.1Гц: %v", u.InputFrequencyHz)
	}
	if u.InputCurrentA != nil {
		t.Fatalf("ток входа -1 → nil, получили %v", *u.InputCurrentA)
	}
	if u.OutputCurrentA == nil || *u.OutputCurrentA != 5.6 {
		t.Fatalf("ток выхода 0.1А: %v", u.OutputCurrentA)
	}
	if u.OutputPowerW == nil || *u.OutputPowerW != 1100 || u.OutputLoadPct == nil || *u.OutputLoadPct != 20 {
		t.Fatalf("мощность/нагрузка: %v / %v", u.OutputPowerW, u.OutputLoadPct)
	}
	if u.RuntimeMin == nil || *u.RuntimeMin != 37 {
		t.Fatalf("время работы: %v", u.RuntimeMin)
	}
	if u.AlarmsPresent == nil || *u.AlarmsPresent != 0 {
		t.Fatalf("аварии: %v", u.AlarmsPresent)
	}
	if u.PolledAt != at {
		t.Fatalf("время опроса: %v", u.PolledAt)
	}
}

func TestDecodeUPSOnBattery(t *testing.T) {
	pdus := ipponPDUs()
	pdus[oidUPSOutputSource] = upsPDU(oidUPSOutputSource, gosnmp.Integer, 5)
	pdus[oidUPSSecondsOnBattery] = upsPDU(oidUPSSecondsOnBattery, gosnmp.Integer, 42)
	pdus[oidUPSBatteryStatus] = upsPDU(oidUPSBatteryStatus, gosnmp.Integer, 3)
	u := upsFromPDUs(pdus, time.Now())
	if u == nil || !u.OnBattery || !u.UPSOnBattery() {
		t.Fatalf("батарея не распознана: %+v", u)
	}
	if u.SecondsOnBattery == nil || *u.SecondsOnBattery != 42 {
		t.Fatalf("секунды на батарее: %v", u.SecondsOnBattery)
	}
}

func TestDecodeUPSNotAUPS(t *testing.T) {
	// Свитч: все UPS-MIB OID → noSuchObject.
	pdus := map[string]gosnmp.SnmpPDU{}
	for _, oid := range upsScalarOIDs {
		pdus[oid] = gosnmp.SnmpPDU{Name: "." + oid, Type: gosnmp.NoSuchObject}
	}
	if u := upsFromPDUs(pdus, time.Now()); u != nil {
		t.Fatalf("свитч не должен давать снимок ИБП: %+v", u)
	}
}

func TestUPSMetricValues(t *testing.T) {
	u := upsFromPDUs(ipponPDUs(), time.Now())
	got := map[string]float32{}
	for _, m := range UPSMetricValues(u) {
		got[m.Type] = m.Value
	}
	want := map[string]float32{
		"ups_battery_charge_pct": 100,
		"ups_runtime_min":        37,
		"ups_battery_voltage_v":  218.7,
		"ups_battery_temp_c":     25,
		"ups_input_voltage_v":    238,
		"ups_output_voltage_v":   220,
		"ups_output_load_pct":    20,
		"ups_output_w":           1100,
		"ups_on_battery":         0,
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s: want %v, got %v (все: %v)", k, v, got[k], got)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("лишние метрики: %v", got)
	}
}

func TestLooksLikeUPS(t *testing.T) {
	cases := []struct {
		cat, descr string
		want       bool
	}{
		{"ups", "", true},
		{"ИБП", "", true},
		{"other", "Network Management Card for UPS", true},
		{"other", "Ippon RT II 6000", true},
		{"other", "Eaton 5PX", true},
		{"switch", "EdgeSwitch 24 Lite", false},
		{"mfu", "KYOCERA ECOSYS M2540dn", false},
	}
	for _, c := range cases {
		if got := LooksLikeUPS(c.cat, c.descr); got != c.want {
			t.Fatalf("LooksLikeUPS(%q,%q)=%v, want %v", c.cat, c.descr, got, c.want)
		}
	}
}

// Живой опрос: UPS_TARGET=192.168.160.177 go test ./internal/snmp/ -run TestReadUPSLive -v
func TestReadUPSLive(t *testing.T) {
	target := os.Getenv("UPS_TARGET")
	if target == "" {
		t.Skip("UPS_TARGET не задан — живой опрос пропущен")
	}
	community := os.Getenv("UPS_COMMUNITY")
	if community == "" {
		community = "public"
	}
	g := &gosnmp.GoSNMP{
		Target:    target,
		Port:      161,
		Community: community,
		Version:   gosnmp.Version2c,
		Timeout:   3 * time.Second,
		Retries:   1,
	}
	if err := g.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer g.Conn.Close()
	u, err := ReadUPS(g, "")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if u == nil {
		t.Fatal("UPS-MIB пуст — узел не похож на ИБП")
	}
	t.Logf("%s: статус=%d (%s), заряд=%s%%, время=%s мин, выход=%s В / %s Вт / %s%%, аварии=%s",
		u.UPSTitle(), u.BatteryStatus, models.UPSBatteryStatusLabel(u.BatteryStatus),
		pf32(u.BatteryChargePct), pi(u.RuntimeMin), pf32(u.OutputVoltageV),
		pf32(u.OutputPowerW), pf32(u.OutputLoadPct), pi(u.AlarmsPresent))

	// Диагностика идентификации: у разных карт строки приходят как string или []byte,
	// часть моделей не отвечает на эти OID — печатаем сырые PDU.
	ident := []string{oidUPSIdentManufacturer, oidUPSIdentModel, oidUPSIdentName, oidUPSIdentAgentSoftware}
	if pkt, err := g.Get(ident); err != nil {
		t.Logf("ident GET: %v", err)
	} else {
		for _, p := range pkt.Variables {
			t.Logf("ident %s type=%v value=%v", p.Name, p.Type, p.Value)
		}
	}
}

func pf32(v *float32) string {
	if v == nil {
		return "—"
	}
	return strconv.FormatFloat(float64(*v), 'f', -1, 32)
}

func pi(v *int) string {
	if v == nil {
		return "—"
	}
	return strconv.Itoa(*v)
}
