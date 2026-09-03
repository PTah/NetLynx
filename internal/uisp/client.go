package uisp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/netutil"
)

// SwitchRow — данные коммутатора из UISP NMS API для импорта в NetLynx.
type SwitchRow struct {
	UISPDeviceID string
	Name         string
	Host         string
	Location     string
}

type siteJSON struct {
	Name   string     `json:"name"`
	Parent *siteJSON  `json:"parent"`
}

type identificationJSON struct {
	ID       string     `json:"id"`
	Role     string     `json:"role"`
	Name     string     `json:"name"`
	Display  string     `json:"displayName"`
	Site     *siteJSON  `json:"site"`
}

// overviewJSON — фрагмент объекта overview в ответе NMS (поле status: active / disconnected и т.д.).
type overviewJSON struct {
	Status string `json:"status"`
}

type deviceJSON struct {
	Identification identificationJSON `json:"identification"`
	Overview         *overviewJSON      `json:"overview"`
	IPAddress        string             `json:"ipAddress"`
}

func sitePath(s *siteJSON) string {
	if s == nil {
		return ""
	}
	var parts []string
	for cur := s; cur != nil; cur = cur.Parent {
		n := strings.TrimSpace(cur.Name)
		if n != "" {
			parts = append(parts, n)
		}
	}
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return strings.Join(parts, " / ")
}

func parseHost(ipField string) string {
	ipField = strings.TrimSpace(ipField)
	if ipField == "" {
		return ""
	}
	if i := strings.Index(ipField, "/"); i >= 0 {
		return strings.TrimSpace(ipField[:i])
	}
	return ipField
}

// NormalizeBaseURL убирает пробелы и завершающий слэш; анти-SSRF (LAN private ок, loopback/metadata — нет).
func NormalizeBaseURL(raw string) (string, error) {
	u := strings.TrimSpace(raw)
	if u == "" {
		return "", fmt.Errorf("пустой URL UISP")
	}
	p, err := url.Parse(u)
	if err != nil || p.Scheme == "" || p.Host == "" {
		return "", fmt.Errorf("некорректный URL UISP (нужен https://хост)")
	}
	if p.Scheme != "https" && p.Scheme != "http" {
		return "", fmt.Errorf("URL UISP: допустимы только http/https")
	}
	u = strings.TrimRight(u, "/")
	if err := netutil.ValidateOutboundURL(u, netutil.UISPPolicy()); err != nil {
		return "", fmt.Errorf("URL UISP: %w", err)
	}
	return u, nil
}

func fetchDevicesArray(ctx context.Context, baseURL, apiToken string) ([]deviceJSON, error) {
	base, err := NormalizeBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	tok := strings.TrimSpace(apiToken)
	if tok == "" {
		return nil, fmt.Errorf("не задан API token UISP")
	}
	reqURL := base + "/nms/api/v2.1/devices"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-auth-token", tok)

	client := netutil.SafeHTTPClient(90*time.Second, netutil.UISPPolicy())
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("запрос к UISP: %w", err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("UISP HTTP %d: %s", res.StatusCode, truncate(string(body), 200))
	}
	var arr []deviceJSON
	if err := json.Unmarshal(body, &arr); err != nil {
		return nil, fmt.Errorf("разбор JSON UISP: %w", err)
	}
	return arr, nil
}

// FetchSwitchOverviewStatuses возвращает для каждого коммутатора (role=switch) UUID → overview.status (как в UISP).
func FetchSwitchOverviewStatuses(ctx context.Context, baseURL, apiToken string) (map[string]string, error) {
	arr, err := fetchDevicesArray(ctx, baseURL, apiToken)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string)
	for _, d := range arr {
		if strings.TrimSpace(strings.ToLower(d.Identification.Role)) != "switch" {
			continue
		}
		id := strings.TrimSpace(d.Identification.ID)
		if id == "" {
			continue
		}
		st := ""
		if d.Overview != nil {
			st = strings.TrimSpace(d.Overview.Status)
		}
		out[id] = st
	}
	return out, nil
}

// FetchSwitches запрашивает /nms/api/v2.1/devices и возвращает узлы с role=switch,
// исключая устройства с overview.status=disconnected (нет связи с UISP).
func FetchSwitches(ctx context.Context, baseURL, apiToken string) ([]SwitchRow, error) {
	arr, err := fetchDevicesArray(ctx, baseURL, apiToken)
	if err != nil {
		return nil, err
	}
	var out []SwitchRow
	for _, d := range arr {
		id := strings.TrimSpace(d.Identification.ID)
		if id == "" {
			continue
		}
		if strings.TrimSpace(strings.ToLower(d.Identification.Role)) != "switch" {
			continue
		}
		if d.Overview != nil && strings.EqualFold(strings.TrimSpace(d.Overview.Status), "disconnected") {
			continue
		}
		host := parseHost(d.IPAddress)
		if host == "" {
			continue
		}
		name := strings.TrimSpace(d.Identification.Display)
		if name == "" {
			name = strings.TrimSpace(d.Identification.Name)
		}
		if name == "" {
			name = host
		}
		loc := sitePath(d.Identification.Site)
		out = append(out, SwitchRow{
			UISPDeviceID: id,
			Name:         name,
			Host:         host,
			Location:     loc,
		})
	}
	return out, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
