package snmp

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/gosnmp/gosnmp"
)

// POWER-ETHERNET-MIB (RFC 3621) — pethPsePortTable.
const (
	basePethPsePortDetectionStatus = "1.3.6.1.2.1.105.1.1.1.6"
	basePethPsePortPhysicalIndex   = "1.3.6.1.2.1.105.1.1.1.16"
)

// HUAWEI-POE-MIB: индекс строки = ifIndex.
const (
	baseHuaweiPoeConsumingPower = "1.3.6.1.4.1.2011.5.25.195.3.1.10" // mW
	baseHuaweiPoePdClass        = "1.3.6.1.4.1.2011.5.25.195.3.1.8" // 1..8 — класс PD
)

// CISCO-POWER-ETHERNET-EXT-MIB: индекс как у peth (часто два или три числа).
const baseCiscoExtPsePortDeviceDetected = "1.3.6.1.4.1.9.9.402.1.2.1.3"

// EdgeSwitch-POWER-ETHERNET-MIB / Broadcom fastPath (Ubiquiti EdgeSwitch): мВт и мА на порту PSE.
const (
	baseFastPathAgentPethOutputPower  = "1.3.6.1.4.1.4413.1.1.15.1.1.1.2"
	baseFastPathAgentPethOutputCurrent = "1.3.6.1.4.1.4413.1.1.15.1.1.1.3"
)

// SNR private MIB (enterprise 40418.7.100.26):
// — На части прошивок (в т.ч. S2989G V705R002) таблица идёт как ...26.10.1.<col>.<port> (walk даёт .5 мВт, .8 on(1)/off(2)).
// — В MIB NAG также описан poePortConfigTable ...26.2.1.<col> / ...26.2.1.1.<col> — на других релизах агент отвечает там.
const (
	baseSNRPoePortCurrentPowerTable10 = "1.3.6.1.4.1.40418.7.100.26.10.1.5"
	baseSNRPoePortPdStatusTable10     = "1.3.6.1.4.1.40418.7.100.26.10.1.8"
	baseSNRPoePortCurrentPower        = "1.3.6.1.4.1.40418.7.100.26.2.1.1.5"
	baseSNRPoePortCurrentPowerAlt     = "1.3.6.1.4.1.40418.7.100.26.2.1.5"
	baseSNRPoePortPdStatus            = "1.3.6.1.4.1.40418.7.100.26.2.1.1.8"
	baseSNRPoePortPdStatusAlt         = "1.3.6.1.4.1.40418.7.100.26.2.1.8"
)

const pethDeliveringPower = 3

func oidSuffixAfterBase(oid, base string) string {
	oid = strings.TrimPrefix(oid, ".")
	base = strings.TrimPrefix(base, ".")
	prefix := base + "."
	if strings.HasPrefix(oid, prefix) {
		return strings.TrimPrefix(oid, prefix)
	}
	return ""
}

// guessIfIndexFromPsePortIndex подбирает ifIndex по номеру порта из индекса PSE (часто совпадает с последним сегментом ifName).
func guessIfIndexFromPsePortIndex(ifRows map[int]IfRow, psePort int) int {
	if psePort <= 0 {
		return 0
	}
	var candidates []int
	for idx, row := range ifRows {
		name := strings.TrimSpace(row.IfName)
		if name == "" {
			continue
		}
		segs := strings.Split(name, "/")
		if len(segs) == 0 {
			continue
		}
		last := segs[len(segs)-1]
		n, err := strconv.Atoi(last)
		if err != nil || n != psePort {
			continue
		}
		candidates = append(candidates, idx)
	}
	return pickBestIfIndexCandidate(ifRows, candidates)
}

// pickBestIfIndexCandidate: при нескольких совпадениях предпочитаем ethernet + «0/N» (EdgeSwitch), не VLAN/SFP-модуль.
func pickBestIfIndexCandidate(ifRows map[int]IfRow, candidates []int) int {
	if len(candidates) == 0 {
		return 0
	}
	if len(candidates) == 1 {
		return candidates[0]
	}
	best := 0
	bestScore := -1
	for _, idx := range candidates {
		row := ifRows[idx]
		score := 0
		if row.IfType == 6 { // ethernetCsmacd
			score += 4
		}
		name := strings.TrimSpace(row.IfName)
		// EdgeSwitch физические порты: 0/1 … 0/48; SFP часто 1/1, 2/1…
		if strings.HasPrefix(name, "0/") {
			score += 3
		}
		if row.Oper == 1 {
			score += 1
		}
		if score > bestScore || (score == bestScore && (best == 0 || idx < best)) {
			bestScore = score
			best = idx
		}
	}
	return best
}

// lastIntFromSuffix возвращает последний целочисленный сегмент индекса OID (например "1.7" -> 7).
func lastIntFromSuffix(suf string) int {
	suf = strings.Trim(suf, ".")
	if suf == "" {
		return 0
	}
	parts := strings.Split(suf, ".")
	for i := len(parts) - 1; i >= 0; i-- {
		n, err := strconv.Atoi(parts[i])
		if err == nil {
			return n
		}
	}
	return 0
}

var trailingPortNumRe = regexp.MustCompile(`(\d+)$`)

// snrPoeIfIndexFromSuffix сопоставляет индекс строки SNR PoE-таблицы с ifIndex.
// Индекс в OID может быть многосегментным; перебираем все числовые сегменты суффикса и выбираем кандидата из ifRows.
func snrPoeIfIndexFromSuffix(suf string, ifRows map[int]IfRow) int {
	suf = strings.Trim(suf, ".")
	if suf == "" {
		return 0
	}
	parts := strings.Split(suf, ".")
	var candidates []int
	seen := make(map[int]struct{})
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n <= 0 {
			continue
		}
		if _, ok := ifRows[n]; ok {
			if _, dup := seen[n]; !dup {
				candidates = append(candidates, n)
				seen[n] = struct{}{}
			}
		}
	}
	if len(candidates) == 0 {
		return guessIfIndexFromPsePortIndex(ifRows, lastIntFromSuffix(suf))
	}
	if len(candidates) == 1 {
		return candidates[0]
	}
	best := 0
	bestScore := -1
	for _, idx := range candidates {
		score := 0
		if ifRows[idx].Oper == 1 {
			score += 2
		}
		name := strings.TrimSpace(ifRows[idx].IfName)
		if m := trailingPortNumRe.FindStringSubmatch(name); len(m) == 2 {
			if pn, err := strconv.Atoi(m[1]); err == nil && pn == lastIntFromSuffix(suf) {
				score += 3
			}
		}
		if score > bestScore || (score == bestScore && (best == 0 || idx < best)) {
			bestScore = score
			best = idx
		}
	}
	return best
}

func walkPethRfc3621(g *gosnmp.GoSNMP, ifRows map[int]IfRow) (map[int]bool, error) {
	det := make(map[string]int64)
	err := g.BulkWalk(basePethPsePortDetectionStatus, func(pdu gosnmp.SnmpPDU) error {
		suf := oidSuffixAfterBase(pdu.Name, basePethPsePortDetectionStatus)
		if suf == "" {
			return nil
		}
		det[suf] = pduInt64(pdu)
		return nil
	})
	if err != nil {
		return nil, err
	}
	phys := make(map[string]int64)
	_ = g.BulkWalk(basePethPsePortPhysicalIndex, func(pdu gosnmp.SnmpPDU) error {
		suf := oidSuffixAfterBase(pdu.Name, basePethPsePortPhysicalIndex)
		if suf == "" {
			return nil
		}
		phys[suf] = pduInt64(pdu)
		return nil
	})

	out := make(map[int]bool)
	for suf, st := range det {
		ifIdx := int(phys[suf])
		if ifIdx <= 0 {
			psePort := lastIntFromSuffix(suf)
			ifIdx = guessIfIndexFromPsePortIndex(ifRows, psePort)
		}
		if ifIdx > 0 {
			// Явный false, если порт есть в PSE-таблице, но не delivering — иначе COALESCE залипает true.
			out[ifIdx] = int(st) == pethDeliveringPower
		}
	}
	return out, nil
}

// Huawei: hwPoePortEntry INDEX = ifIndex; потребление > 0 мВт или класс PD 1..8.
func walkHuaweiPoePort(g *gosnmp.GoSNMP) (map[int]bool, error) {
	out := make(map[int]bool)
	errCon := g.BulkWalk(baseHuaweiPoeConsumingPower, func(pdu gosnmp.SnmpPDU) error {
		suf := oidSuffixAfterBase(pdu.Name, baseHuaweiPoeConsumingPower)
		if suf == "" {
			return nil
		}
		ifIdx, err := strconv.Atoi(suf)
		if err != nil || ifIdx <= 0 {
			return nil
		}
		out[ifIdx] = pduInt64(pdu) > 0
		return nil
	})
	errCls := g.BulkWalk(baseHuaweiPoePdClass, func(pdu gosnmp.SnmpPDU) error {
		suf := oidSuffixAfterBase(pdu.Name, baseHuaweiPoePdClass)
		if suf == "" {
			return nil
		}
		ifIdx, err := strconv.Atoi(suf)
		if err != nil || ifIdx <= 0 {
			return nil
		}
		cl := pduInt64(pdu)
		if cl >= 1 && cl <= 8 {
			if v, ok := out[ifIdx]; ok && !v {
				return nil // 0 мВт важнее класса PD
			}
			out[ifIdx] = true
		}
		return nil
	})
	if errCon != nil && errCls != nil {
		return nil, errCon
	}
	return out, nil
}

func pduTruthTrue(p gosnmp.SnmpPDU) bool {
	switch v := p.Value.(type) {
	case int:
		return v == 1
	case uint:
		return v == 1
	case int64:
		return v == 1
	case uint64:
		return v == 1
	default:
		return pduInt64(p) == 1
	}
}

func walkCiscoExtPseDeviceDetected(g *gosnmp.GoSNMP, ifRows map[int]IfRow) (map[int]bool, error) {
	out := make(map[int]bool)
	err := g.BulkWalk(baseCiscoExtPsePortDeviceDetected, func(pdu gosnmp.SnmpPDU) error {
		detected := pduTruthTrue(pdu)
		suf := oidSuffixAfterBase(pdu.Name, baseCiscoExtPsePortDeviceDetected)
		if suf == "" {
			return nil
		}
		parts := strings.Split(suf, ".")
		if len(parts) == 1 {
			ifIdx, err := strconv.Atoi(parts[0])
			if err == nil && ifIdx > 0 {
				if _, ok := ifRows[ifIdx]; ok {
					if detected {
						out[ifIdx] = true
					} else if _, exists := out[ifIdx]; !exists {
						out[ifIdx] = false
					}
				}
			}
			return nil
		}
		portHint := lastIntFromSuffix(suf)
		if ifIdx := guessIfIndexFromPsePortIndex(ifRows, portHint); ifIdx > 0 {
			if detected {
				out[ifIdx] = true
			} else if _, exists := out[ifIdx]; !exists {
				out[ifIdx] = false
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// parseIfIndexFromBroadcomPethSuffix: индекс в agentPethPsePortTable часто НЕ равен ifIndex — пробуем каждый сегмент суффикса как ifIndex, иначе по номеру порта из ifName.
func parseIfIndexFromBroadcomPethSuffix(suf string, ifRows map[int]IfRow) int {
	suf = strings.Trim(suf, ".")
	if suf == "" {
		return 0
	}
	parts := strings.Split(suf, ".")
	var candidates []int
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n <= 0 {
			continue
		}
		if _, ok := ifRows[n]; ok {
			candidates = append(candidates, n)
		}
	}
	if ifIdx := pickBestIfIndexCandidate(ifRows, candidates); ifIdx > 0 {
		return ifIdx
	}
	return guessIfIndexFromPsePortIndex(ifRows, lastIntFromSuffix(suf))
}

func walkUbiquitiFastPathColumn(g *gosnmp.GoSNMP, ifRows map[int]IfRow, base string, minVal int64) (map[int]bool, error) {
	out := make(map[int]bool)
	err := g.BulkWalk(base, func(pdu gosnmp.SnmpPDU) error {
		suf := oidSuffixAfterBase(pdu.Name, base)
		if suf == "" {
			return nil
		}
		ifIdx := parseIfIndexFromBroadcomPethSuffix(suf, ifRows)
		if ifIdx <= 0 {
			return nil
		}
		if pduInt64(pdu) >= minVal {
			out[ifIdx] = true
		} else if _, exists := out[ifIdx]; !exists {
			out[ifIdx] = false
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func walkUbiquitiFastPathPowerMW(g *gosnmp.GoSNMP, ifRows map[int]IfRow) (map[int]int64, error) {
	out := make(map[int]int64)
	err := g.BulkWalk(baseFastPathAgentPethOutputPower, func(pdu gosnmp.SnmpPDU) error {
		mw := pduInt64(pdu)
		suf := oidSuffixAfterBase(pdu.Name, baseFastPathAgentPethOutputPower)
		if suf == "" {
			return nil
		}
		if ifIdx := parseIfIndexFromBroadcomPethSuffix(suf, ifRows); ifIdx > 0 {
			out[ifIdx] = mw
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func walkHuaweiPoePowerMW(g *gosnmp.GoSNMP) (map[int]int64, error) {
	out := make(map[int]int64)
	err := g.BulkWalk(baseHuaweiPoeConsumingPower, func(pdu gosnmp.SnmpPDU) error {
		suf := oidSuffixAfterBase(pdu.Name, baseHuaweiPoeConsumingPower)
		if suf == "" {
			return nil
		}
		ifIdx, err := strconv.Atoi(suf)
		if err != nil || ifIdx <= 0 {
			return nil
		}
		mw := pduInt64(pdu)
		out[ifIdx] = mw
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// SNR: INDEX=portIndex (в большинстве случаев совпадает с ifIndex физического порта).
// Для SNR важно возвращать и false-статусы, чтобы поллер мог сбрасывать "залипший" poe_active=true.
func walkSNRPoePort(g *gosnmp.GoSNMP, ifRows map[int]IfRow) (map[int]bool, error) {
	out := make(map[int]bool)
	var errs []error
	var walked bool
	bases := []string{
		baseSNRPoePortPdStatusTable10,
		baseSNRPoePortPdStatus, baseSNRPoePortPdStatusAlt,
	}
	for _, base := range bases {
		err := g.BulkWalk(base, func(pdu gosnmp.SnmpPDU) error {
			suf := oidSuffixAfterBase(pdu.Name, base)
			if suf == "" {
				return nil
			}
			ifIdx := snrPoeIfIndexFromSuffix(suf, ifRows)
			if ifIdx <= 0 {
				return nil
			}
			st := pduInt64(pdu)
			// ...26.10.1.8: 1=on, 2=off; ...26.2.1.8: может быть и force(5).
			// Явно проставляем false для off/прочих, чтобы не оставлять старое значение в БД.
			out[ifIdx] = (st == 1 || st == 5)
			return nil
		})
		if err != nil {
			errs = append(errs, err)
			continue
		}
		walked = true
	}
	if !walked && len(errs) > 0 {
		return nil, errs[0]
	}
	return out, nil
}

func walkSNRPoePowerMW(g *gosnmp.GoSNMP, ifRows map[int]IfRow) (map[int]int64, error) {
	out := make(map[int]int64)
	var errs []error
	var walked bool
	bases := []string{
		baseSNRPoePortCurrentPowerTable10,
		baseSNRPoePortCurrentPower, baseSNRPoePortCurrentPowerAlt,
	}
	for _, base := range bases {
		err := g.BulkWalk(base, func(pdu gosnmp.SnmpPDU) error {
			suf := oidSuffixAfterBase(pdu.Name, base)
			if suf == "" {
				return nil
			}
			ifIdx := snrPoeIfIndexFromSuffix(suf, ifRows)
			if ifIdx <= 0 {
				return nil
			}
			mw := pduInt64(pdu)
			// Для SNR записываем и 0, чтобы поллер мог сбрасывать "залипшее" poe_power_w.
			out[ifIdx] = mw
			return nil
		})
		if err != nil {
			errs = append(errs, err)
			continue
		}
		walked = true
	}
	if !walked && len(errs) > 0 {
		return nil, errs[0]
	}
	return out, nil
}

// walkUbiquitiFastPathPoe — Ubiquiti EdgeSwitch (Broadcom fastPath 4413): мощность (мВт) и ток (мА).
func walkUbiquitiFastPathPoe(g *gosnmp.GoSNMP, ifRows map[int]IfRow) (map[int]bool, error) {
	out := make(map[int]bool)
	mp, errP := walkUbiquitiFastPathColumn(g, ifRows, baseFastPathAgentPethOutputPower, 1)
	if errP == nil {
		for k, v := range mp {
			if v {
				out[k] = true
			} else if _, exists := out[k]; !exists {
				out[k] = false
			}
		}
	}
	mc, errC := walkUbiquitiFastPathColumn(g, ifRows, baseFastPathAgentPethOutputCurrent, 3)
	if errC == nil {
		for k, v := range mc {
			if v {
				out[k] = true
			} else if _, exists := out[k]; !exists {
				out[k] = false
			}
		}
	}
	if errP != nil && errC != nil {
		return nil, errP
	}
	return out, nil
}

// WalkPoEPowerWByIfIndex возвращает мощность PoE по ifIndex (в ваттах) из вендорских MIB.
// Если известные источники метрики недоступны, возвращает nil,nil — чтобы не затирать старые значения.
func WalkPoEPowerWByIfIndex(g *gosnmp.GoSNMP, ifRows map[int]IfRow) (map[int]float32, error) {
	outMW := make(map[int]int64)
	var errs []error

	if m, err := walkHuaweiPoePowerMW(g); err != nil {
		errs = append(errs, err)
	} else {
		for ifIdx, mw := range m {
			if mw > 0 {
				outMW[ifIdx] = mw
			} else if _, exists := outMW[ifIdx]; !exists {
				outMW[ifIdx] = 0
			}
		}
	}
	if m, err := walkUbiquitiFastPathPowerMW(g, ifRows); err != nil {
		errs = append(errs, err)
	} else {
		for ifIdx, mw := range m {
			if mw > 0 {
				outMW[ifIdx] = mw
			} else if _, exists := outMW[ifIdx]; !exists {
				outMW[ifIdx] = 0
			}
		}
	}
	if m, err := walkSNRPoePowerMW(g, ifRows); err != nil {
		errs = append(errs, err)
	} else {
		for ifIdx, mw := range m {
			// SNR возвращает текущее потребление в mW; 0 — валидное значение "PoE сейчас не подаётся".
			// Сохраняем и нули, чтобы не оставлять устаревшее ненулевое poe_power_w.
			outMW[ifIdx] = mw
		}
	}

	if len(outMW) == 0 {
		if len(errs) == 3 {
			return nil, fmt.Errorf("poe power: все источники недоступны (Huawei / Ubiquiti 4413 / SNR 40418): %v", errs[0])
		}
		if len(errs) > 0 {
			return nil, nil
		}
		return map[int]float32{}, nil
	}

	outW := make(map[int]float32, len(outMW))
	for ifIdx, mw := range outMW {
		outW[ifIdx] = float32(mw) / 1000
	}
	return outW, nil
}

// WalkPoEDeliveringByIfIndex объединяет PSE-MIB: RFC, Huawei, Cisco, Ubiquiti, SNR.
// LLDP-PD сюда не входит: сосед «я PD» ≠ выдача питания; см. WalkLLDPDot3RemotePDByIfIndex (запасной путь в поллере).
func WalkPoEDeliveringByIfIndex(g *gosnmp.GoSNMP, ifRows map[int]IfRow) (map[int]bool, error) {
	out := make(map[int]bool)
	var errs []error

	merge := func(m map[int]bool) {
		for k, v := range m {
			// true имеет приоритет; false запоминаем только если по порту ещё нет true.
			if v {
				out[k] = true
				continue
			}
			if _, exists := out[k]; !exists {
				out[k] = false
			}
		}
	}

	if m, err := walkPethRfc3621(g, ifRows); err != nil {
		errs = append(errs, err)
	} else {
		merge(m)
	}
	if m, err := walkHuaweiPoePort(g); err != nil {
		errs = append(errs, err)
	} else {
		merge(m)
	}
	if m, err := walkCiscoExtPseDeviceDetected(g, ifRows); err != nil {
		errs = append(errs, err)
	} else {
		merge(m)
	}
	if m, err := walkUbiquitiFastPathPoe(g, ifRows); err != nil {
		errs = append(errs, err)
	} else {
		merge(m)
	}
	if m, err := walkSNRPoePort(g, ifRows); err != nil {
		errs = append(errs, err)
	} else {
		merge(m)
	}

	if len(out) > 0 {
		return out, nil
	}
	if len(errs) == 5 {
		return nil, fmt.Errorf("poe: все 5 обходов MIB завершились ошибкой (RFC105 / Huawei / Cisco / Ubiquiti 4413 / SNR 40418): %v", errs[0])
	}
	// Часть MIB недоступна, при этом ни один источник не подтвердил выдачу PoE —
	// не возвращаем пустую карту: иначе поллер записал бы poe_active=false на все порты
	// и затёр бы NULL/предыдущие значения (например EdgeSwitch 1.9.x-lite без ветки 4413.1.1.15).
	if len(errs) > 0 {
		return nil, nil
	}
	return out, nil
}
