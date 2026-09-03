package store

import (
	"context"
	"net"
	"strconv"
	"strings"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/models"
)

// MACMatchesWiFiPrefix — MAC привязан к IP из подсети (ARP на любом узле).
func (s *Store) MACMatchesWiFiPrefix(ctx context.Context, mac, prefix string) (bool, error) {
	macNorm, ok := FormatFullMAC(mac)
	if !ok {
		return false, nil
	}
	prefix = normalizeWiFiClientIPPrefix(prefix)
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM device_arp_entries
			WHERE lower(mac) = $1
			  AND ip ~ '^[0-9]+\\.[0-9]+\\.[0-9]+\\.[0-9]+$'
			  AND ip::inet << $2::cidr
		)`, strings.ToLower(macNorm), prefix).Scan(&exists)
	return exists, err
}

// ListMACsInIPPrefix — все MAC с ARP-IP в подсети (кэш poller/UI).
func (s *Store) ListMACsInIPPrefix(ctx context.Context, prefix string) (map[string]struct{}, error) {
	prefix = normalizeWiFiClientIPPrefix(prefix)
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT mac FROM device_arp_entries
		WHERE ip ~ '^[0-9]+\\.[0-9]+\\.[0-9]+\\.[0-9]+$'
		  AND ip::inet << $1::cidr`, prefix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]struct{}{}
	for rows.Next() {
		var mac string
		if err := rows.Scan(&mac); err != nil {
			return nil, err
		}
		if norm, ok := FormatFullMAC(mac); ok {
			out[strings.ToLower(norm)] = struct{}{}
		}
	}
	return out, rows.Err()
}

// MACHasARPInPrefix — MAC имеет ARP-IP в подсети WiFi (проверка в Go, без host() в SQL).
func (s *Store) MACHasARPInPrefix(ctx context.Context, mac, prefix string) (bool, error) {
	macNorm, ok := FormatFullMAC(mac)
	if !ok {
		return false, nil
	}
	ips, err := s.ListARPByMAC(ctx, macNorm)
	if err != nil {
		return false, err
	}
	return anyIPInCIDR(ips, prefix), nil
}

func anyIPInCIDR(ips []string, prefix string) bool {
	_, network, err := net.ParseCIDR(normalizeWiFiClientIPPrefix(prefix))
	if err != nil {
		return false
	}
	for _, ipStr := range ips {
		ip := net.ParseIP(strings.TrimSpace(ipStr))
		if ip != nil && network.Contains(ip) {
			return true
		}
	}
	return false
}

// ShouldSkipWiFiMACTracking — не отслеживать MAC WiFi-клиента при выключенной опции.
func (s *Store) ShouldSkipWiFiMACTracking(ctx context.Context, mac string) (bool, error) {
	settings, err := s.GetMACInvestigationSettings(ctx)
	if err != nil {
		return true, err
	}
	if settings.TrackWiFiClients {
		return false, nil
	}
	prefix := settings.WiFiClientIPPrefix
	if inWiFi, err := s.MACHasARPInPrefix(ctx, mac, prefix); err != nil {
		return true, err
	} else if inWiFi {
		return true, nil
	}
	if match, err := s.MACMatchesWiFiPrefix(ctx, mac, prefix); err != nil {
		return true, err
	} else if match {
		return true, nil
	}
	if IsLocallyAdministeredMAC(mac) {
		hasARP, err := s.MACHasARP(ctx, mac)
		if err != nil {
			return true, err
		}
		if !hasARP {
			return true, nil
		}
	}
	return false, nil
}

// MACHasARP — MAC есть в снимках ARP на любом L3-узле.
func (s *Store) MACHasARP(ctx context.Context, mac string) (bool, error) {
	macNorm, ok := FormatFullMAC(mac)
	if !ok {
		return false, nil
	}
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM device_arp_entries WHERE lower(mac) = $1)`,
		strings.ToLower(macNorm)).Scan(&exists)
	return exists, err
}

// IsLocallyAdministeredMAC — IEEE U/L bit в первом октете (типичные random MAC WiFi).
func IsLocallyAdministeredMAC(mac string) bool {
	macNorm, ok := FormatFullMAC(mac)
	if !ok {
		return false
	}
	parts := strings.Split(macNorm, ":")
	if len(parts) != 6 {
		return false
	}
	first, err := strconv.ParseUint(parts[0], 16, 8)
	if err != nil {
		return false
	}
	return byte(first)&0x02 != 0
}

// EventPayloadMAC извлекает MAC из payload события poller.
func EventPayloadMAC(eventType string, payload map[string]interface{}) string {
	if payload == nil {
		return ""
	}
	if m, ok := payload["mac"].(string); ok {
		if norm, ok := FormatFullMAC(m); ok {
			return strings.ToLower(norm)
		}
	}
	switch eventType {
	case "ACCESS_PORT_MAC_SUBSTITUTED":
		for _, k := range []string{"new_mac", "old_mac"} {
			if m, ok := payload[k].(string); ok {
				if norm, ok := FormatFullMAC(m); ok {
					return strings.ToLower(norm)
				}
			}
		}
	}
	return ""
}

// FilterEventsHideWiFiMACs убирает MAC-события WiFi-клиентов из списка (UI/API).
func (s *Store) FilterEventsHideWiFiMACs(ctx context.Context, events []models.Event) ([]models.Event, error) {
	if events == nil {
		events = []models.Event{}
	}
	settings, err := s.GetMACInvestigationSettings(ctx)
	if err != nil {
		return []models.Event{}, err
	}
	if settings.TrackWiFiClients {
		return events, nil
	}
	out := make([]models.Event, 0, len(events))
	for _, ev := range events {
		mac := EventPayloadMAC(ev.EventType, ev.Payload)
		if mac == "" {
			out = append(out, ev)
			continue
		}
		skip, err := s.ShouldSkipWiFiMACTracking(ctx, mac)
		if err != nil {
			return events, err
		}
		if !skip {
			out = append(out, ev)
		}
	}
	return out, nil
}
