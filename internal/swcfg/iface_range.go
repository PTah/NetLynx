package swcfg

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var ubiquitiSlotPort = regexp.MustCompile(`^(\d+)/(\d+)$`)

// CompactUbiquitiInterfaceRange сжимает подряд идущие порты вида 0/4,0/5,0/6 → "0/4-0/7".
// Если сжать нельзя (разные слоты, дыры, не EdgeSwitch-имена) — ok=false.
func CompactUbiquitiInterfaceRange(ifaces []string) (rangeSpec string, ok bool) {
	type sp struct {
		slot, port int
		raw        string
	}
	var list []sp
	for _, raw := range ifaces {
		n := strings.TrimSpace(raw)
		if n == "" {
			return "", false
		}
		m := ubiquitiSlotPort.FindStringSubmatch(n)
		if m == nil {
			return "", false
		}
		slot, _ := strconv.Atoi(m[1])
		port, _ := strconv.Atoi(m[2])
		list = append(list, sp{slot: slot, port: port, raw: n})
	}
	if len(list) < 2 {
		return "", false
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].slot != list[j].slot {
			return list[i].slot < list[j].slot
		}
		return list[i].port < list[j].port
	})
	slot := list[0].slot
	for i := 1; i < len(list); i++ {
		if list[i].slot != slot || list[i].port != list[i-1].port+1 {
			return "", false
		}
	}
	return fmt.Sprintf("%d/%d-%d/%d", slot, list[0].port, slot, list[len(list)-1].port), true
}
