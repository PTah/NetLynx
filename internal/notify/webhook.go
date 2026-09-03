package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/netutil"
)

// Webhook отправляет JSON POST на настроенный URL (интеграции: Slack, n8n, свой скрипт).
type Webhook struct {
	HTTP *http.Client
}

func NewWebhook() *Webhook {
	return &Webhook{
		HTTP: netutil.SafeHTTPClient(10*time.Second, netutil.WebhookPolicy()),
	}
}

func (w *Webhook) PostJSON(ctx context.Context, url string, body any) error {
	if url == "" {
		return fmt.Errorf("пустой webhook URL")
	}
	if err := netutil.ValidateOutboundURL(url, netutil.WebhookPolicy()); err != nil {
		return fmt.Errorf("webhook URL: %w", err)
	}
	jb, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jb))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "NetLynx/0.1")
	client := w.HTTP
	if client == nil {
		client = netutil.SafeHTTPClient(10*time.Second, netutil.WebhookPolicy())
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("webhook HTTP %d", res.StatusCode)
	}
	return nil
}
