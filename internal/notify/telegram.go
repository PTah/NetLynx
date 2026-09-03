package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const maxTelegramRetryAfter = 30 * time.Second

type retryAfterError struct {
	after time.Duration
	msg   string
}

func (e *retryAfterError) Error() string { return e.msg }

func retryAfterOf(err error) (time.Duration, bool) {
	var r *retryAfterError
	if errors.As(err, &r) && r.after > 0 {
		return r.after, true
	}
	return 0, false
}

func capRetryAfter(d time.Duration) time.Duration {
	if d <= 0 {
		return time.Second
	}
	if d > maxTelegramRetryAfter {
		return maxTelegramRetryAfter
	}
	return d
}

func parseRetryAfter(h http.Header, body []byte) time.Duration {
	if s := strings.TrimSpace(h.Get("Retry-After")); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	var parsed struct {
		Parameters struct {
			RetryAfter int `json:"retry_after"`
		} `json:"parameters"`
	}
	if json.Unmarshal(body, &parsed) == nil && parsed.Parameters.RetryAfter > 0 {
		return time.Duration(parsed.Parameters.RetryAfter) * time.Second
	}
	return 0
}

// Telegram отправляет текст через Bot API (sendMessage).
type Telegram struct {
	HTTP *http.Client
}

func NewTelegram() *Telegram {
	return &Telegram{HTTP: &http.Client{Timeout: 10 * time.Second}}
}

func (t *Telegram) SendMessage(ctx context.Context, botToken, chatID, text string) error {
	return t.sendMessage(ctx, botToken, chatID, text, "")
}

// SendHTMLMessage отправляет текст с parse_mode=HTML (поддержка <b> и т.п.).
func (t *Telegram) SendHTMLMessage(ctx context.Context, botToken, chatID, html string) error {
	return t.sendMessage(ctx, botToken, chatID, html, "HTML")
}

func (t *Telegram) sendMessage(ctx context.Context, botToken, chatID, text, parseMode string) error {
	botToken = strings.TrimSpace(botToken)
	chatID = strings.TrimSpace(chatID)
	text = strings.TrimSpace(text)
	if botToken == "" || chatID == "" || text == "" {
		return fmt.Errorf("telegram: пустой token, chat_id или текст")
	}
	apiURL := "https://api.telegram.org/bot" + botToken + "/sendMessage"
	body := map[string]string{
		"chat_id": chatID,
		"text":    text,
	}
	if parseMode != "" {
		body["parse_mode"] = parseMode
	}
	jb, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(jb))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "NetLynx/0.1")
	res, err := t.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 200 && res.StatusCode < 300 {
		return nil
	}
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 8<<10))
	if res.StatusCode == http.StatusTooManyRequests {
		return &retryAfterError{
			after: capRetryAfter(parseRetryAfter(res.Header, raw)),
			msg:   "telegram HTTP 429",
		}
	}
	return fmt.Errorf("telegram HTTP %d", res.StatusCode)
}
