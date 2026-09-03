package investigate

import (
	"context"
	"fmt"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

// Investigator — расширяемое правило поверх MAC-отчёта (фаза 8).
type Investigator interface {
	ID() string
	Investigate(ctx context.Context, st *store.Store, rep *Report) []Hypothesis
}

func DefaultInvestigators() []Investigator {
	return []Investigator{
		RogueMACInvestigator{},
		DuplicateMACInvestigator{},
		PortStormInvestigator{},
	}
}

func mergeHypotheses(base []Hypothesis, extra []Hypothesis) []Hypothesis {
	seen := map[string]struct{}{}
	out := make([]Hypothesis, 0, len(base)+len(extra))
	for _, h := range base {
		if h.ID == "" {
			continue
		}
		if _, ok := seen[h.ID]; ok {
			continue
		}
		seen[h.ID] = struct{}{}
		out = append(out, h)
	}
	for _, h := range extra {
		if h.ID == "" {
			continue
		}
		if _, ok := seen[h.ID]; ok {
			continue
		}
		seen[h.ID] = struct{}{}
		out = append(out, h)
	}
	// если появились «настоящие» гипотезы — убрать insufficient_data
	if len(out) > 1 {
		filtered := out[:0]
		for _, h := range out {
			if h.ID == "insufficient_data" {
				continue
			}
			filtered = append(filtered, h)
		}
		out = filtered
	}
	return out
}

// RogueMACInvestigator — неизвестный MAC на access (событие UNKNOWN_MAC / нет inventory).
type RogueMACInvestigator struct{}

func (RogueMACInvestigator) ID() string { return "rogue_mac" }

func (RogueMACInvestigator) Investigate(ctx context.Context, st *store.Store, rep *Report) []Hypothesis {
	if rep == nil || rep.Identity.MAC == "" {
		return nil
	}
	if rep.Identity.InventoryDeviceID != nil {
		return nil
	}
	accessOnly := false
	for _, f := range rep.Footprint {
		if f.PortRole == "access" || f.PortRole == "" {
			accessOnly = true
			break
		}
	}
	since := time.Now().Add(-24 * time.Hour)
	hasUnknown := false
	if st != nil {
		for _, f := range rep.Footprint {
			if ok, err := st.HasMACEventSince(ctx, f.DeviceID, "UNKNOWN_MAC_ON_ACCESS_PORT", rep.Identity.MAC, since); err == nil && ok {
				hasUnknown = true
				break
			}
		}
		if !hasUnknown {
			seen := map[int64]struct{}{}
			for _, m := range rep.Timeline {
				if _, ok := seen[m.DeviceID]; ok {
					continue
				}
				seen[m.DeviceID] = struct{}{}
				if ok, err := st.HasMACEventSince(ctx, m.DeviceID, "UNKNOWN_MAC_ON_ACCESS_PORT", rep.Identity.MAC, since); err == nil && ok {
					hasUnknown = true
					break
				}
			}
		}
	}
	if !hasUnknown && !(accessOnly && rep.Identity.InventoryDeviceID == nil && len(rep.Footprint) > 0) {
		return nil
	}
	ev := []string{}
	if hasUnknown {
		ev = append(ev, "За 24ч было событие UNKNOWN_MAC_ON_ACCESS_PORT по этому MAC")
	}
	if accessOnly {
		ev = append(ev, "MAC сейчас на access-порту(ах)")
	}
	ev = append(ev, "MAC не совпадает с chassis_mac узлов inventory")
	if !hasUnknown && len(ev) < 2 {
		return nil
	}
	conf := ConfidenceMedium
	if hasUnknown {
		conf = ConfidenceHigh
	}
	return []Hypothesis{{
		ID:         "rogue_mac",
		Confidence: conf,
		Title:      "Возможный rogue / неизвестный MAC на access",
		Evidence:   ev,
		SuggestedChecks: []string{
			"Сверить порт и физическое устройство за ним",
			"Проверить, не spoofing ли это (ARP на шлюзе)",
			"Добавить в inventory или пометить порт ignore/soft, если ожидаемо",
		},
	}}
}

// DuplicateMACInvestigator — один MAC одновременно на разных устройствах/портах.
type DuplicateMACInvestigator struct{}

func (DuplicateMACInvestigator) ID() string { return "duplicate_mac" }

func (DuplicateMACInvestigator) Investigate(_ context.Context, _ *store.Store, rep *Report) []Hypothesis {
	if rep == nil {
		return nil
	}
	devs := map[int64]struct{}{}
	accessDevs := map[int64]int{}
	for _, f := range rep.Footprint {
		devs[f.DeviceID] = struct{}{}
		if f.PortRole == "access" || f.PortRole == "" {
			accessDevs[f.DeviceID]++
		}
	}
	if len(devs) < 2 && len(accessDevs) < 2 {
		// один свитч, несколько access — тоже duplicate/multi
		ports := 0
		for _, f := range rep.Footprint {
			if f.PortRole == "access" || f.PortRole == "" {
				ports++
			}
		}
		if ports < 2 {
			return nil
		}
		return []Hypothesis{{
			ID:         "duplicate_mac",
			Confidence: ConfidenceHigh,
			Title:      "Один MAC на нескольких access-портах одного коммутатора",
			Evidence: []string{
				fmt.Sprintf("Текущий FDB: MAC на %d access-портах", ports),
			},
			SuggestedChecks: []string{
				"Проверить MAC flapping / петлю / dual-homed VM",
				"Admin-down один порт и проверить, исчез ли дубль",
			},
		}}
	}
	if len(accessDevs) >= 2 || len(devs) >= 2 {
		return []Hypothesis{{
			ID:         "duplicate_mac",
			Confidence: ConfidenceHigh,
			Title:      "Дубликат MAC: одновременно в FDB разных узлов",
			Evidence: []string{
				fmt.Sprintf("MAC виден на %d коммутаторах одновременно", len(devs)),
				"Возможен spoofing, клон NIC или L2-петля через чужой сегмент",
			},
			SuggestedChecks: []string{
				"Сверить ARP на шлюзе: один IP — один MAC",
				"Проверить петли (отчёт /investigate/loops)",
				"Найти, какой порт «настоящий» (last_seen / mirror)",
			},
		}}
	}
	return nil
}

// PortStormInvestigator — корреляция с BROADCAST_STORM_* на затронутых узлах.
type PortStormInvestigator struct{}

func (PortStormInvestigator) ID() string { return "port_storm" }

func (PortStormInvestigator) Investigate(ctx context.Context, st *store.Store, rep *Report) []Hypothesis {
	if rep == nil || st == nil {
		return nil
	}
	devIDs := map[int64]struct{}{}
	for _, f := range rep.Footprint {
		devIDs[f.DeviceID] = struct{}{}
	}
	for _, m := range rep.Timeline {
		devIDs[m.DeviceID] = struct{}{}
	}
	if len(devIDs) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(devIDs))
	for id := range devIDs {
		ids = append(ids, id)
	}
	since := time.Now().Add(-6 * time.Hour)
	evs, err := st.ListRecentDeviceEvents(ctx, ids, []string{"BROADCAST_STORM_SUSPECTED", "BROADCAST_STORM_OK"}, since, 20)
	if err != nil || len(evs) == 0 {
		return nil
	}
	suspected := 0
	var samples []string
	for _, e := range evs {
		if e.EventType == "BROADCAST_STORM_SUSPECTED" {
			suspected++
			if len(samples) < 3 {
				samples = append(samples, fmt.Sprintf("device #%d @ %s", e.DeviceID, e.CreatedAt.Format(time.RFC3339)))
			}
		}
	}
	if suspected == 0 {
		return nil
	}
	ev := []string{
		fmt.Sprintf("За 6ч: %d× BROADCAST_STORM_SUSPECTED на узлах, связанных с этим MAC", suspected),
	}
	ev = append(ev, samples...)
	return []Hypothesis{{
		ID:         "port_storm",
		Confidence: ConfidenceMedium,
		Title:      "Корреляция с подозрением на broadcast storm",
		Evidence:   ev,
		SuggestedChecks: []string{
			"Проверить утилизацию портов и рост FDB на затронутых свитчах",
			"Искать L2-петлю: /investigate/loops",
			"Storm-control / STP на access; отключить подозрительный порт",
		},
	}}
}

// applyInvestigators — для тестов и BuildMACReport.
func applyInvestigators(ctx context.Context, st *store.Store, rep *Report, invs []Investigator) {
	if rep == nil {
		return
	}
	var extra []Hypothesis
	for _, inv := range invs {
		if inv == nil {
			continue
		}
		extra = append(extra, inv.Investigate(ctx, st, rep)...)
	}
	rep.Hypotheses = mergeHypotheses(rep.Hypotheses, extra)
}

// stripDuplicateHypothesisID — dual_homed_or_clone перекрывается duplicate_mac investigator.
func preferInvestigatorIDs(hs []Hypothesis) []Hypothesis {
	hasDup := false
	for _, h := range hs {
		if h.ID == "duplicate_mac" {
			hasDup = true
			break
		}
	}
	if !hasDup {
		return hs
	}
	out := hs[:0]
	for _, h := range hs {
		if h.ID == "dual_homed_or_clone" {
			continue
		}
		out = append(out, h)
	}
	return out
}
