package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/backup"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/config"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/live"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Server struct {
	st                    *store.Store
	cfg                   config.Config
	bi                    BuildInfo
	hub                   *live.Hub
	traps                 TrapListener
	mux                   chi.Router
	loginLimit            *loginLimiter
	sseTickets            *sseTicketStore
	backupRun             *backup.Runner
	wifiFilterInvalidator wifiFilterCacheInvalidator
	runningConfigCache    sync.Map // int64 → deviceRunningConfigEntry
}

// wifiFilterCacheInvalidator — poller сбрасывает кэш WiFi MAC после PATCH настроек.
type wifiFilterCacheInvalidator interface {
	InvalidateWiFiFilterCache()
}

type BuildInfo struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	BuiltAt string `json:"built_at"`
}

func NewServer(st *store.Store, cfg config.Config, bi BuildInfo, hub *live.Hub, traps TrapListener, pollPauser backup.PollPauser) *Server {
	s := &Server{
		st:         st,
		cfg:        cfg,
		bi:         bi,
		hub:        hub,
		traps:      traps,
		loginLimit: newLoginLimiter(),
		sseTickets: newSSETicketStore(),
	}
	s.backupRun = backup.NewRunner(nil, st, cfg)
	if pollPauser != nil {
		s.backupRun.SetPollPauser(pollPauser)
		if wfi, ok := pollPauser.(wifiFilterCacheInvalidator); ok {
			s.wifiFilterInvalidator = wfi
		}
	}
	s.backupRun.RecoverInterrupted(context.Background())
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	// RealIP только при TrustProxy — иначе XFF спуфит RemoteAddr и ломает rate-limit.
	if cfg.TrustProxy {
		r.Use(middleware.RealIP)
	}
	r.Use(middleware.Recoverer)
	r.Use(securityHeaders)
	r.Use(requestBodyLimit)
	r.Use(cors.Handler(cors.Options{
		// Для разработки (Vite :5173) и для работы с одного порта (:8080).
		AllowedOrigins: []string{
			"http://localhost:5173",
			"http://127.0.0.1:5173",
			"http://localhost:8080",
			"http://127.0.0.1:8080",
		},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Content-Type", "Authorization", "X-Confirm"},
		AllowCredentials: false,
	}))

	r.Get("/health", s.handleHealth)
	// SSE: EventSource не шлёт Authorization — вход по одноразовому ticket (или Bearer для клиентов с заголовком).
	r.Get("/api/v1/events/stream", s.handleEventsStream)
	r.Get("/api/v1/settings/journal/stream", s.handleJournalStream)

	authR := chi.NewRouter()
	authR.Post("/login", s.handleAuthLogin)
	authR.Post("/refresh", s.handleAuthRefresh)
	authR.Post("/logout", s.handleAuthLogout)
	authR.With(s.authMiddleware).Get("/me", s.handleAuthMe)
	authR.With(s.authMiddleware).Post("/sse-ticket", s.handleIssueSSETicket)
	r.Mount("/api/v1/auth", authR)

	ar := chi.NewRouter()
	ar.Use(s.authMiddleware)

	ar.Group(func(r chi.Router) {
		r.Use(s.requireMinRole(roleViewer))
		r.Get("/devices", s.handleListDevices)
		r.Get("/devices/{id}/detail", s.handleGetDeviceDetail)
		r.Get("/devices/{id}/events", s.handleListDeviceEvents)
		r.Get("/devices/{id}/metrics", s.handleDeviceMetrics)
		r.Get("/devices/{id}/traffic-series", s.handleDeviceTrafficSeries)
		r.Get("/devices/{id}", s.handleGetDevice)
		r.Get("/devices/{id}/interfaces", s.handleListInterfaces)
		r.Get("/devices/{id}/interfaces/{ifIndex}/clients", s.handleListPortClients)
		r.Get("/devices/{id}/interfaces/{ifIndex}/settings", s.handleGetPortSettings)
		r.Get("/devices/{id}/interfaces/{ifIndex}/shut-impact", s.handlePortShutImpact)
		r.Get("/ports/search", s.handleSearchPorts)
		r.Get("/investigate/mac", s.handleInvestigateMAC)
		r.Get("/investigate/mac/flappers", s.handleInvestigateMACFlappers)
		r.Get("/investigate/mac/fdb-history", s.handleInvestigateMACFDBHistory)
		r.Get("/investigate/loops", s.handleInvestigateLoops)
		r.Get("/postmortem", s.handleGetPostmortem)
		r.Get("/devices/{id}/config/snapshots", s.handleListDeviceConfigSnapshots)
		r.Get("/devices/{id}/config/snapshots/{snapId}", s.handleGetDeviceConfigSnapshot)
		r.Get("/devices/{id}/config/diff", s.handleDeviceConfigDiff)
		r.Get("/devices/{id}/vlans", s.handleGetDeviceVLANs)
		r.Get("/devices/{id}/fdb/snapshots", s.handleListDeviceFDBSnapshots)
		r.Get("/events", s.handleListEvents)
		r.Get("/system/stats", s.handleSystemStats)
		r.Get("/topology", s.handleGetTopology)
		r.Get("/discovered", s.handleListDiscovered)
		r.Get("/manual-links", s.handleListManualLinks)
		r.Get("/settings/topology", s.handleGetTopologySettings)
		r.Get("/settings/device-categories", s.handleListDeviceCategories)
		r.Get("/settings/inventory/stale-fdb", s.handleListStaleLiveFDB)
		r.Get("/settings/inventory/offline-devices", s.handleListOfflineDevices)
		r.Get("/settings/mac-investigation", s.handleGetMACInvestigationSettings)
	})

	ar.Group(func(r chi.Router) {
		r.Use(s.requireMinRole(roleOperator))
		r.Post("/devices/import-uisp", s.handleImportUISP)
		r.Post("/devices", s.handleCreateDevice)
		r.Patch("/devices/{id}/name", s.handlePatchDeviceName)
		r.Patch("/devices/{id}/host", s.handlePatchDeviceHost)
		r.Patch("/devices/{id}/chassis-mac", s.handlePatchDeviceChassisMAC)
		r.Patch("/devices/{id}/online-override", s.handlePatchDeviceOnlineOverride)
		r.Patch("/devices/{id}/trust-link-traps", s.handlePatchDeviceTrustLinkTraps)
		r.Patch("/devices/{id}/location", s.handlePatchDeviceLocation)
		r.Patch("/devices/{id}/category", s.handlePatchDeviceCategory)
		r.Patch("/devices/{id}/poll-interval", s.handlePatchDevicePollInterval)
		r.Patch("/devices/{id}/monitoring", s.handlePatchDeviceMonitoring)
		r.Patch("/devices/{id}/interfaces/{ifIndex}/thresholds", s.handlePatchPortThresholds)
		r.Patch("/devices/{id}/interfaces/{ifIndex}/descr", s.handlePatchPortDescr)
		r.Patch("/investigate/mac/status", s.handlePatchMACInvestigationStatus)
		r.Patch("/devices/{id}/interfaces/{ifIndex}/admin", s.handlePatchPortAdmin)
		r.Patch("/devices/{id}/interfaces/{ifIndex}/poe", s.handlePatchPortPoE)
		r.Patch("/devices/{id}/interfaces/{ifIndex}/isolate", s.handlePatchPortIsolate)
		r.Patch("/devices/{id}/interfaces/{ifIndex}/dhcp-snooping", s.handlePatchPortDHCPSnoop)
		r.Patch("/devices/{id}/interfaces/{ifIndex}/flow-control", s.handlePatchPortFlowControl)
		r.Patch("/devices/{id}/interfaces/{ifIndex}/stp", s.handlePatchPortSTP)
		r.Patch("/devices/{id}/interfaces/{ifIndex}/vlan", s.handlePatchPortVLAN)
		r.Post("/devices/{id}/vlans", s.handleCreateDeviceVLAN)
		r.Delete("/devices/{id}/vlans", s.handleDeleteDeviceVLANs)
		r.Patch("/devices/{id}/vlans/{vlanId}", s.handlePatchDeviceVLAN)
		r.Delete("/devices/{id}/vlans/{vlanId}", s.handleDeleteDeviceVLAN)
		r.Put("/devices/{id}/interfaces/{ifIndex}/ignore", s.handlePutPortIgnore)
		r.Delete("/devices/{id}/interfaces/{ifIndex}/ignore", s.handleDeletePortIgnore)
		r.Get("/devices/{id}/port-ignores", s.handleListPortIgnores)
		r.Patch("/devices/{id}", s.handlePatchDevice)
		r.Patch("/devices/{id}/ssh", s.handlePatchDeviceSSH)
		r.Delete("/devices/{id}", s.handleDeleteDevice)
		r.Post("/devices/{id}/snmp-test", s.handleSNMPTest)
		r.Post("/devices/{id}/traceroute", s.handleDeviceTraceroute)
		r.Post("/devices/{id}/tcp-probe", s.handleDeviceTCPProbe)
		r.Post("/devices/{id}/sync-port-roles-from-config", s.handleSyncPortRolesFromConfig)
		r.Post("/devices/{id}/config/snapshot", s.handleFetchDeviceConfigSnapshot)
		r.Post("/devices/{id}/interfaces/{ifIndex}/clients/preview", s.handlePreviewPortClient)
		r.Post("/devices/{id}/interfaces/{ifIndex}/clients/promote", s.handlePromotePortClient)
		r.Post("/discovered/{id}/ignore", s.handleIgnoreDiscovered)
		r.Post("/discovered/{id}/reopen", s.handleReopenDiscovered)
		r.Post("/discovered/{id}/preview", s.handlePreviewDiscovered)
		r.Post("/discovered/{id}/promote", s.handlePromoteDiscovered)
		r.Post("/manual-links", s.handleCreateManualLink)
		r.Patch("/manual-links/{id}", s.handlePatchManualLink)
		r.Delete("/manual-links/{id}", s.handleDeleteManualLink)
		r.Get("/settings/notifications", s.handleGetNotifications)
		r.Patch("/settings/notifications", s.handlePatchNotifications)
		r.Post("/settings/notifications/email-test", s.handlePostEmailTest)
		r.Get("/settings/uisp", s.handleGetUISPSettings)
		r.Patch("/settings/uisp", s.handlePatchUISPSettings)
		r.Patch("/settings/topology", s.handlePatchTopologySettings)
		r.Patch("/settings/mac-investigation", s.handlePatchMACInvestigationSettings)
		r.Get("/settings/snmp-traps", s.handleGetSNMPTrapSettings)
		r.Patch("/settings/snmp-traps", s.handlePatchSNMPTrapSettings)
		r.Get("/settings/snmp-traps/logs", s.handleListSNMPTrapLogs)
		r.Delete("/settings/snmp-traps/logs", s.handleClearSNMPTrapLogs)
		r.Post("/settings/device-categories", s.handleCreateDeviceCategory)
		r.Patch("/settings/device-categories/{id}", s.handlePatchDeviceCategoryDef)
		r.Delete("/settings/device-categories/{id}", s.handleDeleteDeviceCategory)
		r.Post("/settings/inventory/stale-fdb/clear", s.handleClearStaleLiveFDB)
	})

	ar.Group(func(r chi.Router) {
		r.Use(s.requireMinRole(roleAdmin))
		r.Delete("/devices", s.handleDeleteAllDevices)
		r.Post("/settings/inventory/offline-devices/delete", s.handleDeleteOfflineDevices)
		r.Get("/users", s.handleListUsers)
		r.Post("/users", s.handleCreateUser)
		r.Patch("/users/{id}", s.handlePatchUser)
		r.Delete("/users/{id}", s.handleDeleteUser)
		r.Get("/audit", s.handleListAudit)
		r.Get("/settings/backup", s.handleGetBackupSettings)
		r.Patch("/settings/backup", s.handlePatchBackupSettings)
		r.Post("/backup/run", s.handleRunBackup)
		r.Get("/backup/archives", s.handleListBackupArchives)
		r.Post("/backup/verify", s.handleVerifyBackup)
		r.Post("/backup/import", s.handleImportBackup)
		r.Get("/settings/journal", s.handleJournalMeta)
		r.Get("/settings/journal/lines", s.handleJournalLines)
	})

	r.Mount("/api/v1", ar)

	r.With(s.authMiddleware, s.requireMinRole(roleViewer)).Get("/metrics", func(w http.ResponseWriter, req *http.Request) {
		metricUptimeSeconds.Set(time.Since(startedAt).Seconds())
		promhttp.Handler().ServeHTTP(w, req)
	})

	if cfg.WebStaticDir != "" {
		r.Handle("/*", spaDir(cfg.WebStaticDir))
	} else {
		r.Get("/", s.handleRootHint)
	}

	s.mux = r
	metricBuildInfo.WithLabelValues(s.bi.Version, s.bi.Commit).Set(1)
	return s
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) handleRootHint(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<!DOCTYPE html><html lang="ru"><head><meta charset="utf-8"><title>NetLynx</title></head><body>
<h1>NetLynx</h1>
<p>Сервер запущен, но <strong>веб-интерфейс ещё не собран</strong> (нет папки <code>web/dist</code>).</p>
<p>Сделайте по инструкции в README раздел «Собрать веб один раз», затем перезапустите программу.</p>
<p>Пока можно смотреть сырой JSON: <a href="/api/v1/devices">/api/v1/devices</a>, <a href="/api/v1/events">/api/v1/events</a>.</p>
</body></html>`))
}
