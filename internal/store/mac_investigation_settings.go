package store

import (
	"context"
	"net"
	"strings"

	"github.com/jackc/pgx/v5"
)

const defaultWiFiClientIPPrefix = "192.168.120.0/24"

// MACInvestigationSettings — глобальные настройки расследования MAC (id=1).
type MACInvestigationSettings struct {
	TrackWiFiClients   bool   `json:"track_wifi_clients"`
	WiFiClientIPPrefix string `json:"wifi_client_ip_prefix"`
}

func normalizeWiFiClientIPPrefix(prefix string) string {
	p := strings.TrimSpace(prefix)
	if p == "" {
		return defaultWiFiClientIPPrefix
	}
	return p
}

// DefaultWiFiClientIPPrefix — CIDR WiFi-клиентов по умолчанию (без настроек из БД).
func DefaultWiFiClientIPPrefix() string {
	return defaultWiFiClientIPPrefix
}

// ValidateWiFiClientIPPrefix проверяет CIDR для WiFi-клиентов.
func ValidateWiFiClientIPPrefix(prefix string) error {
	p := normalizeWiFiClientIPPrefix(prefix)
	if _, _, err := net.ParseCIDR(p); err != nil {
		return err
	}
	return nil
}

func (s *Store) GetMACInvestigationSettings(ctx context.Context) (MACInvestigationSettings, error) {
	var out MACInvestigationSettings
	err := s.pool.QueryRow(ctx, `
		SELECT track_wifi_clients, wifi_client_ip_prefix
		FROM mac_investigation_settings WHERE id = 1`,
	).Scan(&out.TrackWiFiClients, &out.WiFiClientIPPrefix)
	if err == pgx.ErrNoRows {
		return MACInvestigationSettings{WiFiClientIPPrefix: defaultWiFiClientIPPrefix}, nil
	}
	if err != nil {
		return MACInvestigationSettings{}, err
	}
	out.WiFiClientIPPrefix = normalizeWiFiClientIPPrefix(out.WiFiClientIPPrefix)
	return out, nil
}

func (s *Store) UpsertMACInvestigationSettings(ctx context.Context, in MACInvestigationSettings) error {
	prefix := normalizeWiFiClientIPPrefix(in.WiFiClientIPPrefix)
	if err := ValidateWiFiClientIPPrefix(prefix); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO mac_investigation_settings (id, track_wifi_clients, wifi_client_ip_prefix, updated_at)
		VALUES (1, $1, $2, now())
		ON CONFLICT (id) DO UPDATE SET
			track_wifi_clients = EXCLUDED.track_wifi_clients,
			wifi_client_ip_prefix = EXCLUDED.wifi_client_ip_prefix,
			updated_at = now()`,
		in.TrackWiFiClients, prefix)
	return err
}

// WiFiTrackingExcludePrefix — CIDR для исключения WiFi-клиентов или nil, если отслеживать всё.
func (s *Store) WiFiTrackingExcludePrefix(ctx context.Context) (*string, error) {
	settings, err := s.GetMACInvestigationSettings(ctx)
	if err != nil {
		return nil, err
	}
	if settings.TrackWiFiClients {
		return nil, nil
	}
	p := settings.WiFiClientIPPrefix
	return &p, nil
}
