package poller

import (
	"testing"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/models"
)

func TestShouldEmitDeviceOnline(t *testing.T) {
	now := time.Date(2026, 8, 23, 9, 5, 0, 0, time.UTC)
	hours := func(h time.Duration) *time.Time {
		t := now.Add(-h)
		return &t
	}
	sec := func(s time.Duration) *time.Time {
		t := now.Add(-s)
		return &t
	}

	cases := []struct {
		name         string
		wasOnline    bool
		streak       offlineStreakState
		offlineSince *time.Time
		pollSec      int
		hasOffEvent  bool
		want         bool
	}{
		{name: "still online", wasOnline: true, want: false},
		{
			name:      "confirmed offline in this process",
			wasOnline: false,
			streak:    offlineStreakState{n: offlineConfirmPolls, fromOnline: true},
			want:      true,
		},
		{
			name:         "one-poll blip: no offline event, short offline_since",
			wasOnline:    false,
			streak:       offlineStreakState{n: 1, fromOnline: true},
			offlineSince: sec(20 * time.Second),
			pollSec:      60,
			want:         false,
		},
		{
			name:         "102s SNMP blip at 60s poll — no DEVICE_OFFLINE, no ONLINE",
			wasOnline:    false,
			streak:       offlineStreakState{n: 1, fromOnline: true},
			offlineSince: sec(102 * time.Second),
			pollSec:      60,
			want:         false,
		},
		{
			name:         "weekend recovery after restart, OFFLINE already in DB",
			wasOnline:    false,
			streak:       offlineStreakState{},
			offlineSince: hours(40 * time.Hour),
			pollSec:      60,
			hasOffEvent:  true,
			want:         true,
		},
		{
			name:         "restart mid-outage 8min, no DEVICE_OFFLINE event — no lone ONLINE",
			wasOnline:    false,
			streak:       offlineStreakState{},
			offlineSince: sec(8*time.Minute + 50*time.Second),
			pollSec:      120,
			hasOffEvent:  false,
			want:         false,
		},
		{
			name:         "restart while down for 4 minutes at 60s poll, had OFFLINE",
			wasOnline:    false,
			streak:       offlineStreakState{},
			offlineSince: sec(4 * time.Minute),
			pollSec:      60,
			hasOffEvent:  true,
			want:         true,
		},
		{
			name:      "new device first success",
			wasOnline: false,
			streak:    offlineStreakState{},
			want:      false,
		},
		{
			name:         "already-offline at start, down 30s",
			wasOnline:    false,
			streak:       offlineStreakState{},
			offlineSince: sec(30 * time.Second),
			want:         false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldEmitDeviceOnline(tc.wasOnline, tc.streak, tc.offlineSince, now, tc.pollSec, tc.hasOffEvent)
			if got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestShouldEmitDeviceOfflineNow(t *testing.T) {
	// Обычный путь: SNMP-сбой, ICMP не менялся — ждём второй опрос.
	if shouldEmitDeviceOfflineNow(offlineStreakState{n: 1, fromOnline: true}, false, 2, true) {
		t.Fatal("первый сбой без пропажи ICMP не должен шуметь")
	}
	if !shouldEmitDeviceOfflineNow(offlineStreakState{n: offlineConfirmPolls, fromOnline: true}, false, 2, true) {
		t.Fatal("второй подряд сбой — OFFLINE")
	}
	// ICMP отвечал и замолчал — сообщаем сразу (критичные/важные).
	if !shouldEmitDeviceOfflineNow(offlineStreakState{n: 1, fromOnline: true}, true, 2, true) {
		t.Fatal("пропажа ICMP + SNMP = OFFLINE на первом опросе")
	}
	// Телефон/камера: instant ping-dead выключен — ждём 3 опроса.
	if shouldEmitDeviceOfflineNow(offlineStreakState{n: 1, fromOnline: true}, true, 3, false) {
		t.Fatal("мягкий узел: ICMP-флап на первом опросе не шлёт OFFLINE")
	}
	if shouldEmitDeviceOfflineNow(offlineStreakState{n: 2, fromOnline: true}, false, 3, false) {
		t.Fatal("мягкий узел: второй сбой ещё не порог")
	}
	if !shouldEmitDeviceOfflineNow(offlineStreakState{n: 3, fromOnline: true}, false, 3, false) {
		t.Fatal("мягкий узел: третий подряд — OFFLINE")
	}
	// Без эпизода online→offline (после рестарта) — не шумим.
	if shouldEmitDeviceOfflineNow(offlineStreakState{n: 1}, true, 2, true) {
		t.Fatal("нет перехода online→offline — OFFLINE не шлём")
	}
	// Повторные опросы в том же эпизоде не дублируют событие.
	if shouldEmitDeviceOfflineNow(offlineStreakState{n: offlineConfirmPolls + 1, fromOnline: true}, true, 2, true) {
		t.Fatal("повтор OFFLINE в том же эпизоде")
	}
}

func TestOfflineConfirmForCategory(t *testing.T) {
	if offlineConfirmForCategory("switch") != 2 || offlineConfirmForCategory("nas") != 2 {
		t.Fatal("критичные/NAS: confirm=2")
	}
	if offlineConfirmForCategory("phone") != 3 || offlineConfirmForCategory("ap") != 3 {
		t.Fatal("AP/телефон: confirm=3")
	}
	if offlineConfirmForCategory("camera") != 3 || offlineConfirmForCategory("mfu") != 3 {
		t.Fatal("камера/МФУ: confirm=3")
	}
	if allowInstantPingDead("switch") != true || allowInstantPingDead("phone") != false {
		t.Fatal("instant ping-dead только для критичных/важных")
	}
}

func TestPingWentDead(t *testing.T) {
	yes, no := true, false
	dev := func(ping *bool) *models.Device { return &models.Device{LastPingOK: ping} }
	if !pingWentDead(dev(&no), dev(&yes)) {
		t.Fatal("был ping → пропал ping = пропажа из сети")
	}
	if pingWentDead(dev(&yes), dev(&yes)) {
		t.Fatal("ping жив")
	}
	if pingWentDead(dev(&no), dev(&no)) {
		t.Fatal("ICMP закрыт всегда (напр. коммутатор без ICMP) — не наш случай")
	}
	if pingWentDead(dev(&no), nil) || pingWentDead(nil, dev(&yes)) {
		t.Fatal("нет предыдущего состояния — не считаем пропажей")
	}
	if pingWentDead(dev(&no), dev(nil)) {
		t.Fatal("неизвестный предыдущий ping — не считаем пропажей")
	}
}

func TestShouldCatchUpDeviceOffline(t *testing.T) {
	now := time.Date(2026, 8, 23, 12, 1, 0, 0, time.UTC)
	since := now.Add(-8*time.Minute - 50*time.Second)
	if !shouldCatchUpDeviceOffline(false, offlineStreakState{}, &since, now, 120, false) {
		t.Fatal("restart mid-outage still down: catch-up OFFLINE")
	}
	if shouldCatchUpDeviceOffline(false, offlineStreakState{}, &since, now, 120, true) {
		t.Fatal("already have OFFLINE event")
	}
	if shouldCatchUpDeviceOffline(true, offlineStreakState{}, &since, now, 120, false) {
		t.Fatal("this process saw online→offline: wait for confirm polls")
	}
	if shouldCatchUpDeviceOffline(false, offlineStreakState{fromOnline: true, n: 1}, &since, now, 120, false) {
		t.Fatal("confirm in progress")
	}
}
