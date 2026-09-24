package snmp

import (
	"sort"
	"strconv"
	"strings"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/models"
	"github.com/gosnmp/gosnmp"
)

// Printer-MIB (RFC 3805) — общий стандарт для Kyocera, Sharp и большинства МФУ.
const (
	oidPrtMarkerLifeCount        = "1.3.6.1.2.1.43.10.2.1.4"
	oidPrtMarkerLifeCountDefault = "1.3.6.1.2.1.43.10.2.1.4.1.1"
	oidPrtMarkerSuppliesColorant = "1.3.6.1.2.1.43.11.1.1.3"
	oidPrtMarkerSuppliesClass    = "1.3.6.1.2.1.43.11.1.1.4"
	oidPrtMarkerSuppliesType     = "1.3.6.1.2.1.43.11.1.1.5"
	oidPrtMarkerSuppliesDescr    = "1.3.6.1.2.1.43.11.1.1.6"
	oidPrtMarkerSuppliesUnit     = "1.3.6.1.2.1.43.11.1.1.7"
	oidPrtMarkerSuppliesMax      = "1.3.6.1.2.1.43.11.1.1.8"
	oidPrtMarkerSuppliesLevel    = "1.3.6.1.2.1.43.11.1.1.9"
	oidPrtMarkerColorantValue    = "1.3.6.1.2.1.43.12.1.1.4"

	// Kyocera private fallbacks (enterprise 1347), если IETF отдаёт -2/-3.
	oidKyoceraPagesA       = "1.3.6.1.4.1.1347.42.2.1.1.1.1"
	oidKyoceraPagesB       = "1.3.6.1.4.1.1347.42.3.1.1.1.1"
	oidKyoceraTonerBlack   = "1.3.6.1.4.1.1347.40.10.1.1.3.1"
	oidKyoceraTonerCyan    = "1.3.6.1.4.1.1347.40.10.1.1.3.2"
	oidKyoceraTonerMagenta = "1.3.6.1.4.1.1347.40.10.1.1.3.3"
	oidKyoceraTonerYellow  = "1.3.6.1.4.1.1347.40.10.1.1.3.4"

	oidSharpPages = "1.3.6.1.4.1.2385.1.1.4.3.1.3.4.1.4.1"

	prtSuppliesTypeToner          = 3
	prtSuppliesTypeWasteToner     = 4
	prtSuppliesTypeTonerCartridge = 21
	prtSuppliesClassConsumed      = 3
	prtSuppliesUnitPercent        = 18

	prtLevelOther   = -1
	prtLevelUnknown = -2
	prtLevelSome    = -3
)

// PrinterReading — снимок Printer-MIB: life count + тонерные картриджи.
type PrinterReading struct {
	PageCount *int64
	Toners    []models.PrinterToner
}

func (p *PrinterReading) HasData() bool {
	return p != nil && (p.PageCount != nil || len(p.Toners) > 0)
}

// LooksLikePrinter — МФУ/принтер по категории узла или sysDescr.
func LooksLikePrinter(category, sysDescr string) bool {
	c := strings.ToLower(strings.TrimSpace(category))
	if c == "mfu" || c == "мфу" || c == "принтер" || c == "printer" || c == "mfp" {
		return true
	}
	d := strings.ToLower(sysDescr)
	needles := []string{
		"printer", "mfp", "mfu", "multifunction", "laserjet",
		"kyocera", "ecosys", "taskalfa",
		"xerox", "workcentre", "versalink",
		"sharp mx", "sharp bp", "sharp dx",
		"canon i-sensys", "imagerunner",
		"brother dcp", "brother mfc",
		"hp color laser", "officejet",
	}
	for _, n := range needles {
		if strings.Contains(d, n) {
			return true
		}
	}
	return false
}

// TonerMetricType — metric_samples.metric_type для графика остатка.
func TonerMetricType(key string) string {
	if key == "" {
		key = "other"
	}
	return "toner_" + key + "_pct"
}

// ReadPrinter читает prtMarkerLifeCount и prtMarkerSuppliesTable.
// Индексы CMYK не зашиты: цвет берётся из prtMarkerColorantValue и description.
func ReadPrinter(g *gosnmp.GoSNMP, sysDescr string) (*PrinterReading, error) {
	if g == nil {
		return nil, nil
	}
	out := &PrinterReading{}

	pages := readPageCount(g)
	if pages == nil {
		pages = readVendorPageCount(g, sysDescr)
	}
	out.PageCount = pages

	out.Toners = readTonerSupplies(g)
	if needsVendorToner(out.Toners) && strings.Contains(strings.ToLower(sysDescr), "kyocera") {
		applyKyoceraTonerFallback(g, &out.Toners)
	}

	if !out.HasData() {
		return nil, nil
	}
	return out, nil
}

func readPageCount(g *gosnmp.GoSNMP) *int64 {
	vals := walkIntMap(g, oidPrtMarkerLifeCount)
	if v := pickPageCount(vals); v != nil {
		return v
	}
	if v, ok := getInt64(g, oidPrtMarkerLifeCountDefault); ok && v >= 0 {
		return &v
	}
	return nil
}

func pickPageCount(vals map[string]int64) *int64 {
	var best *int64
	for _, v := range vals {
		if v < 0 {
			continue
		}
		if best == nil || v > *best {
			x := v
			best = &x
		}
	}
	return best
}

func readVendorPageCount(g *gosnmp.GoSNMP, sysDescr string) *int64 {
	d := strings.ToLower(sysDescr)
	oids := []string{}
	if strings.Contains(d, "kyocera") || strings.Contains(d, "ecosys") || strings.Contains(d, "taskalfa") {
		oids = append(oids, oidKyoceraPagesA, oidKyoceraPagesB)
	}
	if strings.Contains(d, "sharp") {
		oids = append(oids, oidSharpPages)
	}
	for _, oid := range oids {
		if v, ok := getInt64(g, oid); ok && v >= 0 {
			return &v
		}
	}
	return nil
}

func readTonerSupplies(g *gosnmp.GoSNMP) []models.PrinterToner {
	descr := walkStringMap(g, oidPrtMarkerSuppliesDescr)
	if len(descr) == 0 {
		return nil
	}
	class := walkIntMap(g, oidPrtMarkerSuppliesClass)
	typ := walkIntMap(g, oidPrtMarkerSuppliesType)
	unit := walkIntMap(g, oidPrtMarkerSuppliesUnit)
	maxv := walkIntMap(g, oidPrtMarkerSuppliesMax)
	level := walkIntMap(g, oidPrtMarkerSuppliesLevel)
	colorantIdx := walkIntMap(g, oidPrtMarkerSuppliesColorant)
	colorantVal := walkStringMap(g, oidPrtMarkerColorantValue)

	var out []models.PrinterToner
	for suf, name := range descr {
		t := typ[suf]
		if t == prtSuppliesTypeWasteToner || isWasteDescr(name) {
			continue
		}
		if t != 0 && t != prtSuppliesTypeToner && t != prtSuppliesTypeTonerCartridge {
			if !looksLikeTonerDescr(name) {
				continue
			}
		}
		if c, ok := class[suf]; ok && c != 0 && c != prtSuppliesClassConsumed {
			continue
		}
		key := classifyTonerKey(name)
		if ci, ok := colorantIdx[suf]; ok && ci > 0 {
			if cv := colorantValueForSupply(suf, ci, colorantVal); cv != "" {
				if k := classifyTonerKey(cv); k != "other" {
					key = k
				}
			}
		}
		u := unit[suf]
		lv, hasLevel := level[suf]
		mx, hasMax := maxv[suf]
		var levelPtr, maxPtr *int64
		if hasLevel {
			x := lv
			levelPtr = &x
		}
		if hasMax && mx >= 0 {
			x := mx
			maxPtr = &x
		}
		pct, unknown, remainingOK := tonerPercent(lv, hasLevel, mx, hasMax, u)
		out = append(out, models.PrinterToner{
			Key:         key,
			Label:       tonerLabel(key, name),
			Description: strings.TrimSpace(name),
			Level:       levelPtr,
			Max:         maxPtr,
			Pct:         pct,
			Unit:        int(u),
			Unknown:     unknown,
			RemainingOK: remainingOK,
			MetricType:  TonerMetricType(key),
		})
	}
	return sortToners(out)
}

func colorantValueForSupply(supplySuffix string, colorantIndex int64, colorants map[string]string) string {
	hr := strings.SplitN(supplySuffix, ".", 2)[0]
	idx := strconv.FormatInt(colorantIndex, 10)
	candidates := []string{
		hr + "." + idx,
		idx,
	}
	for _, c := range candidates {
		if v, ok := colorants[c]; ok && strings.TrimSpace(v) != "" {
			return v
		}
	}
	for suf, v := range colorants {
		if strings.HasSuffix(suf, "."+idx) || suf == idx {
			return v
		}
	}
	return ""
}

func needsVendorToner(toners []models.PrinterToner) bool {
	if len(toners) == 0 {
		return true
	}
	for _, t := range toners {
		if t.Pct == nil {
			return true
		}
	}
	return false
}

func applyKyoceraTonerFallback(g *gosnmp.GoSNMP, toners *[]models.PrinterToner) {
	type kv struct {
		key, oid, label string
	}
	fallbacks := []kv{
		{"black", oidKyoceraTonerBlack, "Чёрный"},
		{"cyan", oidKyoceraTonerCyan, "Голубой"},
		{"magenta", oidKyoceraTonerMagenta, "Пурпурный"},
		{"yellow", oidKyoceraTonerYellow, "Жёлтый"},
	}
	byKey := map[string]int{}
	for i, t := range *toners {
		byKey[t.Key] = i
	}
	for _, fb := range fallbacks {
		v, ok := getInt64(g, fb.oid)
		if !ok || v < 0 || v > 100 {
			continue
		}
		pct := float32(v)
		if i, exists := byKey[fb.key]; exists {
			if (*toners)[i].Pct == nil {
				(*toners)[i].Pct = &pct
				(*toners)[i].Unknown = false
				(*toners)[i].RemainingOK = false
			}
			continue
		}
		*toners = append(*toners, models.PrinterToner{
			Key:         fb.key,
			Label:       fb.label,
			Description: fb.label,
			Pct:         &pct,
			Unit:        prtSuppliesUnitPercent,
			MetricType:  TonerMetricType(fb.key),
		})
	}
	*toners = sortToners(*toners)
}

// tonerPercent: если unit=percent — берём current; иначе current/max*100.
// -2 неизвестно, -3 «есть остаток» без точного значения.
func tonerPercent(level int64, hasLevel bool, max int64, hasMax bool, unit int64) (pct *float32, unknown, remainingOK bool) {
	if !hasLevel {
		if hasMax && max > 0 {
			return nil, true, false
		}
		return nil, true, false
	}
	if level == prtLevelUnknown || level == prtLevelOther {
		return nil, true, false
	}
	if level == prtLevelSome {
		return nil, false, true
	}
	if level < 0 {
		return nil, true, false
	}
	if unit == prtSuppliesUnitPercent {
		if hasMax && max > 0 && max != 100 && level <= max {
			v := float32(level) * 100 / float32(max)
			return clampPct(v), false, false
		}
		return clampPct(float32(level)), false, false
	}
	if hasMax && max > 0 {
		return clampPct(float32(level) * 100 / float32(max)), false, false
	}
	return nil, true, false
}

func clampPct(v float32) *float32 {
	if v < 0 {
		v = 0
	}
	if v > 100 {
		v = 100
	}
	return &v
}

func classifyTonerKey(s string) string {
	l := strings.ToLower(strings.TrimSpace(s))
	l = strings.ReplaceAll(l, "-", " ")
	switch {
	case strings.Contains(l, "cyan") || strings.Contains(l, "голубой") || strings.Contains(l, "синий"):
		return "cyan"
	case strings.Contains(l, "magenta") || strings.Contains(l, "пурпур") || strings.Contains(l, "малинов"):
		return "magenta"
	case strings.Contains(l, "yellow") || strings.Contains(l, "жёлт") || strings.Contains(l, "желт"):
		return "yellow"
	case strings.Contains(l, "black") || strings.Contains(l, "чёрн") || strings.Contains(l, "черн"):
		return "black"
	case l == "c" || strings.HasPrefix(l, "c ") || strings.Contains(l, " c "):
		return "cyan"
	case l == "m" || strings.HasPrefix(l, "m ") || strings.Contains(l, " m "):
		return "magenta"
	case l == "y" || strings.HasPrefix(l, "y ") || strings.Contains(l, " y "):
		return "yellow"
	case l == "k" || strings.HasPrefix(l, "k "):
		return "black"
	default:
		return "other"
	}
}

func tonerLabel(key, descr string) string {
	switch key {
	case "black":
		return "Чёрный"
	case "cyan":
		return "Голубой"
	case "magenta":
		return "Пурпурный"
	case "yellow":
		return "Жёлтый"
	default:
		if s := strings.TrimSpace(descr); s != "" {
			return s
		}
		return "Тонер"
	}
}

func isWasteDescr(s string) bool {
	l := strings.ToLower(s)
	return strings.Contains(l, "waste") || strings.Contains(l, "отход")
}

func looksLikeTonerDescr(s string) bool {
	l := strings.ToLower(s)
	return strings.Contains(l, "toner") || strings.Contains(l, "тонер") ||
		strings.Contains(l, "cartridge") || strings.Contains(l, "картридж") ||
		classifyTonerKey(s) != "other"
}

func sortToners(in []models.PrinterToner) []models.PrinterToner {
	order := map[string]int{"black": 0, "cyan": 1, "magenta": 2, "yellow": 3}
	out := append([]models.PrinterToner(nil), in...)
	sort.SliceStable(out, func(i, j int) bool {
		oi, oki := order[out[i].Key]
		oj, okj := order[out[j].Key]
		if !oki {
			oi = 10
		}
		if !okj {
			oj = 10
		}
		if oi != oj {
			return oi < oj
		}
		return out[i].Description < out[j].Description
	})
	return out
}

func walkIntMap(g *gosnmp.GoSNMP, base string) map[string]int64 {
	out := map[string]int64{}
	_ = printerWalk(g, base, func(pdu gosnmp.SnmpPDU) error {
		suf, ok := oidSuffix(pdu.Name, base)
		if !ok || suf == "" {
			return nil
		}
		if pdu.Type == gosnmp.NoSuchObject || pdu.Type == gosnmp.NoSuchInstance || pdu.Type == gosnmp.Null {
			return nil
		}
		out[suf] = pduInt64(pdu)
		return nil
	})
	return out
}

func walkStringMap(g *gosnmp.GoSNMP, base string) map[string]string {
	out := map[string]string{}
	_ = printerWalk(g, base, func(pdu gosnmp.SnmpPDU) error {
		suf, ok := oidSuffix(pdu.Name, base)
		if !ok || suf == "" {
			return nil
		}
		s := strings.TrimSpace(pduString(pdu))
		if s != "" {
			out[suf] = s
		}
		return nil
	})
	return out
}

func printerWalk(g *gosnmp.GoSNMP, base string, fn func(gosnmp.SnmpPDU) error) error {
	if g.Version == gosnmp.Version1 {
		return g.Walk(base, fn)
	}
	return g.BulkWalk(base, fn)
}

func oidSuffix(oid, base string) (string, bool) {
	oid = normalizeOID(oid)
	base = normalizeOID(base)
	if oid == base {
		return "", true
	}
	pref := base + "."
	if !strings.HasPrefix(oid, pref) {
		return "", false
	}
	return oid[len(pref):], true
}

func getInt64(g *gosnmp.GoSNMP, oid string) (int64, bool) {
	pdus, err := g.Get([]string{oid})
	if err != nil || pdus == nil || len(pdus.Variables) == 0 {
		return 0, false
	}
	p := pdus.Variables[0]
	if p.Type == gosnmp.NoSuchObject || p.Type == gosnmp.NoSuchInstance || p.Type == gosnmp.Null {
		return 0, false
	}
	return pduInt64(p), true
}
