package notify

import (
	"strings"
	"testing"
	"time"
)

func TestBuildAlertEmailSingleOnline(t *testing.T) {
	t.Parallel()
	mail := BuildAlertEmail("papatramp@example.com", []EmailDeviceCard{{
		DeviceID:   7,
		Title:      "Voevodina-PC",
		Subtitle:   "192.168.160.50 · Компьютеры",
		Detail:     "Офис УК - Приемная",
		Attachment: "на коммутаторе «EdgeSwitch 48 #10 (Berloga)», порт 0/12",
		StatusLine: "Компьютер снова в сети с 8:58 после 16ч 18м 37с отсутствия в сети",
		Kind:       "DEVICE_ONLINE",
		Category:   "computer",
		URL:        "http://192.168.160.121:8080/devices/7",
	}}, time.Date(2026, 8, 21, 8, 58, 0, 0, time.Local))
	if !strings.Contains(mail.Subject, "онлайн") {
		t.Fatalf("subject: %s", mail.Subject)
	}
	if !strings.Contains(mail.Subject, "Voevodina-PC") {
		t.Fatalf("subject should include device name: %s", mail.Subject)
	}
	if !strings.Contains(mail.HTMLBody, "Voevodina-PC") {
		t.Fatal("missing device name")
	}
	if !strings.Contains(mail.HTMLBody, "192.168.160.50") {
		t.Fatal("missing host/IP")
	}
	if !strings.Contains(mail.HTMLBody, "Офис УК - Приемная") {
		t.Fatal("missing location")
	}
	if !strings.Contains(mail.HTMLBody, "EdgeSwitch 48 #10") {
		t.Fatal("missing attachment switch")
	}
	if !strings.Contains(mail.HTMLBody, "Оповещение NetLynx") {
		t.Fatal("missing title")
	}
	if !strings.Contains(mail.HTMLBody, "cid:netlynx-logo") {
		t.Fatal("missing logo cid")
	}
	if strings.Contains(mail.HTMLBody, "See Full Report") || strings.Contains(mail.HTMLBody, "Full Report") {
		t.Fatal("must not include Full Report button")
	}
	if strings.Contains(mail.HTMLBody, "▣") || strings.Contains(mail.HTMLBody, "f1f5f9") {
		t.Fatal("must not include placeholder device icon")
	}
	if !strings.Contains(mail.HTMLBody, "cid:netlynx-icon-computer") {
		t.Fatal("missing device category icon cid")
	}
	// HTML: только приветствие, без дублирующего intro
	if strings.Contains(mail.HTMLBody, "ушло в оффлайн.") || strings.Contains(mail.HTMLBody, "снова в сети.") {
		// статус в карточке содержит «снова в сети с» — это ок; intro «снова в сети.» — нет
		if strings.Contains(mail.HTMLBody, "Устройство «Voevodina-PC»") {
			t.Fatal("HTML must not duplicate intro paragraph")
		}
	}
	if !strings.Contains(mail.HTMLBody, "/devices/7") {
		t.Fatal("missing device link")
	}
	if !strings.Contains(mail.HTMLBody, "Здравствуйте, papatramp") {
		t.Fatalf("greeting: %s", mail.HTMLBody)
	}
	if !strings.Contains(mail.TextBody, "Здравствуйте, papatramp") {
		t.Fatalf("text greeting: %s", mail.TextBody)
	}
}

func TestDeviceIconEmbedded(t *testing.T) {
	t.Parallel()
	if len(DeviceIconPNG("switch")) < 500 {
		t.Fatal("switch icon missing")
	}
	if len(DeviceIconPNG("unknown-xyz")) < 500 {
		t.Fatal("fallback other icon missing")
	}
	if DeviceIconCID("Camera") != "netlynx-icon-camera" {
		t.Fatal(DeviceIconCID("Camera"))
	}
	if DeviceIconCID("ilo") != "netlynx-icon-ilo-idrac-ipmi" {
		t.Fatal(DeviceIconCID("ilo"))
	}
	if len(DeviceIconPNG("ilo")) < 500 {
		t.Fatal("ilo icon missing")
	}
	if len(DeviceIconPNG("industrial")) < 500 {
		t.Fatal("industrial icon missing")
	}
	cards := []EmailDeviceCard{{Category: "switch"}, {Category: "switch"}, {Category: "ap"}}
	inl := CollectDeviceIconInline(cards)
	if len(inl) != 2 {
		t.Fatalf("unique icons: got %d", len(inl))
	}
}

func TestBuildAlertEmailMultiOffline(t *testing.T) {
	t.Parallel()
	mail := BuildAlertEmail("ops@ex.com", []EmailDeviceCard{
		{DeviceID: 1, Title: "A", Subtitle: "a-pc", StatusLine: "Ушло в оффлайн в 9:00", Kind: "DEVICE_OFFLINE", URL: "http://x/devices/1"},
		{DeviceID: 2, Title: "B", Subtitle: "b-pc", StatusLine: "Ушло в оффлайн в 9:01", Kind: "DEVICE_OFFLINE", URL: "http://x/devices/2"},
	}, time.Now())
	if !strings.Contains(mail.Subject, "2 устройств оффлайн") {
		t.Fatalf("subject: %s", mail.Subject)
	}
	// intro больше не в HTML
	if strings.Contains(mail.HTMLBody, "2 устройств из списка узлов") {
		t.Fatalf("HTML should not have intro: %s", mail.HTMLBody)
	}
	if !strings.Contains(mail.HTMLBody, ">A<") && !strings.Contains(mail.HTMLBody, "A</div>") {
		t.Fatal("missing card A")
	}
	if !strings.Contains(mail.HTMLBody, "cid:netlynx-icon-other") {
		t.Fatal("missing default category icon")
	}
}

func TestFormatOfflineDuration(t *testing.T) {
	t.Parallel()
	if formatOfflineDuration(58517) != "16ч 15м 17с" {
		t.Fatal(formatOfflineDuration(58517))
	}
}

func TestCardStatusLineOnlineWithDuration(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 8, 25, 8, 50, 0, 0, time.Local)
	got := cardStatusLine("DEVICE_ONLINE", "computer", map[string]interface{}{
		"offline_duration_sec": 14*3600 + 50*60 + 3,
	}, at)
	want := "Компьютер снова в сети с 08:50 после 14ч 50м 3с отсутствия в сети"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestLogoEmbedded(t *testing.T) {
	t.Parallel()
	if len(LogoPNG()) < 1000 {
		t.Fatal("logo missing")
	}
}
