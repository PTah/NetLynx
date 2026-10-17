package poller

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/config"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

func testEngine() *Engine {
	return &Engine{
		cfg: config.Config{
			CriticalPollInterval: 15 * time.Second,
			HeavyPollInterval:    5 * time.Minute,
			PollSchedulerSeconds: 10,
		},
		log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func TestPollPriority(t *testing.T) {
	cases := map[string]int{
		"router":   pollPriorityCritical,
		"switch":   pollPriorityCritical,
		"server":   pollPriorityCritical,
		"ups":      pollPriorityImportant,
		"nas":      pollPriorityImportant,
		"computer": pollPriorityImportant,
		"ilo":      pollPriorityImportant,
		"phone":    pollPriorityNormal,
		"camera":   pollPriorityNormal,
		"mfu":      pollPriorityNormal,
		"":         pollPriorityNormal,
	}
	for cat, want := range cases {
		if got := pollPriority(cat); got != want {
			t.Fatalf("pollPriority(%q)=%d, want %d", cat, got, want)
		}
	}
	// Критичные типы должны идти раньше остальных в очереди опроса.
	if pollPriority("switch") >= pollPriority("ups") || pollPriority("ups") >= pollPriority("phone") {
		t.Fatal("порядок приоритетов нарушен")
	}
}

func TestShouldSchedulePollCriticalCap(t *testing.T) {
	e := testEngine()
	now := time.Now()
	ago := func(sec int) *time.Time { t0 := now.Add(-time.Duration(sec) * time.Second); return &t0 }

	// Коммутатор в карточке 60 с, но лёгкий опрос — не реже 15 с.
	sw := store.PollDevice{ID: 1, DeviceCategory: "switch", PollIntervalSeconds: 60, LastPollAt: ago(20)}
	if !e.shouldSchedulePoll(&sw, now) {
		t.Fatal("коммутатор должен опрашиваться по критичному интервалу (15 с), а не по 60")
	}
	if got := e.pollIntervalOf(&sw); got != 15*time.Second {
		t.Fatalf("интервал коммутатора: %v, want 15s", got)
	}
	notYet := store.PollDevice{ID: 2, DeviceCategory: "switch", PollIntervalSeconds: 60, LastPollAt: ago(5)}
	if e.shouldSchedulePoll(&notYet, now) {
		t.Fatal("5 секунд назад опрашивали — рано")
	}

	// Телефон не критичен: интервал из карточки (60 с) соблюдается.
	phoneDue := store.PollDevice{ID: 3, DeviceCategory: "phone", PollIntervalSeconds: 60, LastPollAt: ago(20)}
	if e.shouldSchedulePoll(&phoneDue, now) {
		t.Fatal("телефон не должен опрашиваться чаще своего интервала")
	}
	phoneOld := store.PollDevice{ID: 4, DeviceCategory: "phone", PollIntervalSeconds: 60, LastPollAt: ago(61)}
	if !e.shouldSchedulePoll(&phoneOld, now) {
		t.Fatal("телефон просрочен — пора")
	}

	// Если в карточке интервал меньше критичного, берём интервал карточки.
	fast := store.PollDevice{ID: 5, DeviceCategory: "switch", PollIntervalSeconds: 10, LastPollAt: ago(11)}
	if !e.shouldSchedulePoll(&fast, now) {
		t.Fatal("10-секундный интервал должен работать")
	}
	if got := e.pollIntervalOf(&fast); got != 10*time.Second {
		t.Fatalf("интервал: %v, want 10s", got)
	}
}

func TestShouldPollHeavy(t *testing.T) {
	e := testEngine()
	now := time.Now()
	ago := func(d time.Duration) *time.Time { t0 := now.Add(-d); return &t0 }

	if !e.shouldPollHeavy(&store.PollDevice{ID: 1}, now) {
		t.Fatal("тяжёлый опрос ещё не делали — пора")
	}
	recent := store.PollDevice{ID: 2, LastHeavyPollAt: ago(2 * time.Minute)}
	if e.shouldPollHeavy(&recent, now) {
		t.Fatal("2 минуты назад — рано (по умолчанию 5 минут)")
	}
	old := store.PollDevice{ID: 3, LastHeavyPollAt: ago(6 * time.Minute)}
	if !e.shouldPollHeavy(&old, now) {
		t.Fatal("6 минут — пора")
	}
	// Персональный интервал узла важнее глобального.
	heavy900 := 900
	custom := store.PollDevice{ID: 4, HeavyPollIntervalSeconds: &heavy900, LastHeavyPollAt: ago(10 * time.Minute)}
	if e.shouldPollHeavy(&custom, now) {
		t.Fatal("у узла 15 минут — 10 минут мало")
	}
	// nil = интервал по умолчанию из настроек (5 минут).
	def := store.PollDevice{ID: 5, LastHeavyPollAt: ago(6 * time.Minute)}
	if !e.shouldPollHeavy(&def, now) {
		t.Fatal("без персонального интервала работает POLL_HEAVY_SECONDS")
	}
}

// Лёгкий опрос критичного узла не должен зависеть от тяжёлого: интервалы независимы.
func TestCriticalIntervalIndependentOfHeavy(t *testing.T) {
	e := testEngine()
	now := time.Now()
	lastHeavy := now.Add(-time.Minute)
	sw := store.PollDevice{
		ID: 1, DeviceCategory: "switch", PollIntervalSeconds: 60,
		LastPollAt:      func() *time.Time { t0 := now.Add(-16 * time.Second); return &t0 }(),
		LastHeavyPollAt: &lastHeavy,
	}
	if e.shouldPollHeavy(&sw, now) {
		t.Fatal("тяжёлый опрос только что был — не пора")
	}
	if !e.shouldSchedulePoll(&sw, now) {
		t.Fatal("лёгкий опрос всё равно должен идти по 15 с")
	}
}

// Утилизация/скорость пишутся в историю не чаще MetricsUtilInterval, даже если опрос идёт
// каждые 15 с: иначе metric_samples растёт до десятков миллионов строк в сутки.
func TestUtilSampleThrottle(t *testing.T) {
	e := testEngine()
	e.cfg.MetricsUtilInterval = 60 * time.Second
	now := time.Now()
	const id int64 = 7

	if !e.utilSampleDue(id, now) {
		t.Fatal("первый сэмпл должен писаться")
	}
	if e.utilSampleDue(id, now.Add(15*time.Second)) {
		t.Fatal("через 15 с при интервале 60 с — рано")
	}
	if e.utilSampleDue(id, now.Add(45*time.Second)) {
		t.Fatal("через 45 с — всё ещё рано")
	}
	if !e.utilSampleDue(id, now.Add(61*time.Second)) {
		t.Fatal("через 61 с — пора")
	}
	// Если троттлинг выключен (интервал 0) — пишем каждый опрос.
	e.cfg.MetricsUtilInterval = 0
	if !e.utilSampleDue(99, now) || !e.utilSampleDue(99, now) {
		t.Fatal("при нулевом интервале писать нужно всегда")
	}
}

// Узел без SNMP (камера/ПК/ТВ): после 3 сбоев подряд SNMP не опрашивается,
// ICMP-достижимость остаётся — иначе такие узлы жгут слоты таймаутами.
func TestSNMPBackoff(t *testing.T) {
	e := testEngine()
	now := time.Now()
	const id int64 = 42

	if e.snmpBackoffActive(id, now) {
		t.Fatal("без сбоев паузы быть не должно")
	}
	for i := 1; i < snmpFailThreshold; i++ {
		e.noteSNMPFailure(id, now)
		if e.snmpBackoffActive(id, now) {
			t.Fatalf("пауза не должна включаться после %d сбоев", i)
		}
	}
	st := e.noteSNMPFailure(id, now)
	if !e.snmpBackoffActive(id, now) {
		t.Fatalf("после %d сбоев должна быть пауза", st.failures)
	}
	// Пауза истекает по времени.
	if e.snmpBackoffActive(id, now.Add(snmpBackoffPause+time.Second)) {
		t.Fatal("пауза должна закончиться")
	}
	// Успешный опрос сбрасывает счётчик.
	e.clearSNMPFailures(id)
	if e.snmpBackoffActive(id, now) {
		t.Fatal("после успеха паузы быть не должно")
	}
	// Второй эпизод: снова 3 сбоя → снова пауза.
	for i := 0; i < snmpFailThreshold; i++ {
		e.noteSNMPFailure(id, now.Add(time.Hour))
	}
	if !e.snmpBackoffActive(id, now.Add(time.Hour)) {
		t.Fatal("повторный эпизод должен ставить паузу")
	}
}
