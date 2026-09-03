package poller

import (
	"context"
	"strings"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

const wifiFilterCacheTTL = 2 * time.Minute

type wifiFilterCache struct {
	trackWiFi bool
	prefix    string
	macs      map[string]struct{}
	loadedAt  time.Time
}

// shouldTrackMAC — false для WiFi-клиентов (ARP в wifi_client_ip_prefix), если track_wifi_clients выключен.
func (e *Engine) shouldTrackMAC(ctx context.Context, mac string) bool {
	norm, ok := store.FormatFullMAC(mac)
	if !ok {
		return true
	}
	norm = strings.ToLower(norm)
	cache := e.loadWiFiFilterCache(ctx)
	if cache.trackWiFi {
		return true
	}
	if _, hit := cache.macs[norm]; hit {
		return false
	}
	inWiFi, err := e.st.MACHasARPInPrefix(ctx, norm, cache.prefix)
	if err != nil {
		e.log.Warn("wifi mac prefix check", "mac", norm, "err", err)
		return false
	}
	if inWiFi {
		e.rememberWiFiMAC(norm)
		return false
	}
	match, err := e.st.MACMatchesWiFiPrefix(ctx, norm, cache.prefix)
	if err != nil {
		e.log.Warn("wifi mac prefix sql", "mac", norm, "err", err)
		return false
	}
	if match {
		e.rememberWiFiMAC(norm)
		return false
	}
	if store.IsLocallyAdministeredMAC(norm) {
		hasARP, arpErr := e.st.MACHasARP(ctx, norm)
		if arpErr != nil {
			e.log.Warn("wifi mac arp presence", "mac", norm, "err", arpErr)
			return false
		}
		if !hasARP {
			return false
		}
	}
	return true
}

func (e *Engine) rememberWiFiMAC(norm string) {
	e.wifiFilterMu.Lock()
	defer e.wifiFilterMu.Unlock()
	if e.wifiFilterCache.macs == nil {
		e.wifiFilterCache.macs = map[string]struct{}{}
	}
	e.wifiFilterCache.macs[norm] = struct{}{}
}

func (e *Engine) loadWiFiFilterCache(ctx context.Context) wifiFilterCache {
	e.wifiFilterMu.Lock()
	defer e.wifiFilterMu.Unlock()
	if time.Since(e.wifiFilterCache.loadedAt) < wifiFilterCacheTTL && !e.wifiFilterCache.loadedAt.IsZero() {
		return e.wifiFilterCache
	}
	settings, err := e.st.GetMACInvestigationSettings(ctx)
	if err != nil {
		e.log.Warn("mac investigation settings", "err", err)
		e.wifiFilterCache = wifiFilterCache{
			trackWiFi: false,
			prefix:    store.DefaultWiFiClientIPPrefix(),
			macs:      map[string]struct{}{},
			loadedAt:  time.Now(),
		}
		return e.wifiFilterCache
	}
	prefix := settings.WiFiClientIPPrefix
	if settings.TrackWiFiClients {
		e.wifiFilterCache = wifiFilterCache{trackWiFi: true, prefix: prefix, loadedAt: time.Now()}
		return e.wifiFilterCache
	}
	macs, err := e.st.ListMACsInIPPrefix(ctx, prefix)
	if err != nil {
		e.log.Warn("wifi mac prefix list", "err", err)
		macs = map[string]struct{}{}
	}
	e.wifiFilterCache = wifiFilterCache{
		trackWiFi: false,
		prefix:    prefix,
		macs:      macs,
		loadedAt:  time.Now(),
	}
	return e.wifiFilterCache
}

func (e *Engine) InvalidateWiFiFilterCache() {
	e.invalidateWiFiFilterCache()
}

func (e *Engine) invalidateWiFiFilterCache() {
	e.wifiFilterMu.Lock()
	e.wifiFilterCache = wifiFilterCache{}
	e.wifiFilterMu.Unlock()
}
