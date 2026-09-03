package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/api"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/config"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/configsnapshot"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/fdbsnapshot"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/live"
	ddb "git.kalinamall.ru/PapaTramp/netlynx/internal/db"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/notify"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/poller"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/syslogrecv"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/traprecv"
)

var (
	version = "0.19.3"
	commit  = "none"
	builtAt = "unknown"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("конфиг", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := ddb.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("postgres", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := ddb.Migrate(ctx, pool); err != nil {
		slog.Error("миграции", "err", err)
		os.Exit(1)
	}

	st := store.New(pool)
	if err := st.RefreshAllOfflineSince(ctx); err != nil {
		slog.Warn("offline_since backfill", "err", err)
	}
	if !cfg.AuthDisabled {
		secret, err := st.EnsureJWTSecret(ctx, cfg.JWTSecret, cfg.AdminPassword)
		if err != nil {
			slog.Error("jwt secret", "err", err)
			os.Exit(1)
		}
		cfg.JWTSecret = secret
		if !cfg.CookieSecure {
			slog.Warn("NETLYNX_COOKIE_SECURE=false — refresh cookie без Secure (только для HTTP/LAN)")
		}
	}
	if cfg.AdminPassword != "" {
		if err := st.EnsureAdminUser(ctx, cfg.AdminUser, cfg.AdminPassword); err != nil {
			slog.Error("auth bootstrap", "err", err)
			os.Exit(1)
		}
	}
	hub := live.NewHub()
	hook := notify.NewEventHook(slog.Default(), st, notify.NewWebhook(), notify.HookOptions{
		PublicBaseURL: cfg.PublicBaseURL,
	})
	eng := poller.New(slog.Default(), st, cfg, hook, hub)

	var workers sync.WaitGroup
	workers.Add(1)
	go func() {
		defer workers.Done()
		eng.Run(ctx)
	}()

	trapMgr := traprecv.NewManager(slog.Default(), st, hub, cfg.SNMPTrapCommunity, hook, eng.TrapLinkIncidentAction)
	if err := trapMgr.Reload(ctx); err != nil {
		slog.Error("snmp trap receiver start", "err", err)
		os.Exit(1)
	}
	defer trapMgr.Stop()

	if cfg.SyslogListenAddr != "" {
		sysRecv := syslogrecv.New(slog.Default(), st, cfg.SyslogListenAddr, eng.EmitMACFlappingFromSyslog)
		workers.Add(1)
		go func() {
			defer workers.Done()
			if err := sysRecv.Run(ctx); err != nil && ctx.Err() == nil {
				slog.Error("syslog receiver", "err", err)
			}
		}()
	}
	apiSrv := api.NewServer(st, cfg, api.BuildInfo{
		Version: version,
		Commit:  commit,
		BuiltAt: builtAt,
	}, hub, trapMgr, eng)
	workers.Add(1)
	go func() {
		defer workers.Done()
		apiSrv.RunBackupScheduler(ctx)
	}()

	cfgSnapSched := configsnapshot.NewScheduler(slog.Default(), st, cfg)
	workers.Add(1)
	go func() {
		defer workers.Done()
		cfgSnapSched.Run(ctx)
	}()

	fdbSnapSched := fdbsnapshot.NewScheduler(slog.Default(), st, cfg)
	workers.Add(1)
	go func() {
		defer workers.Done()
		fdbSnapSched.Run(ctx)
	}()

	httpSrv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           apiSrv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(shCtx); err != nil {
			slog.Warn("http shutdown", "err", err)
		}
	}()

	_, listenAddr := trapMgr.Status()
	slog.Info("netlynxd",
		"listen", cfg.HTTPAddr,
		"poll_scheduler_sec", cfg.PollSchedulerSeconds,
		"access_port_long_idle", cfg.AccessPortLongIdle.String(),
		"trap_listen", listenAddr,
		"syslog_listen", cfg.SyslogListenAddr,
		"version", version,
		"commit", commit,
		"built_at", builtAt,
	)
	if cfg.WebStaticDir != "" {
		h := cfg.HTTPAddr
		if strings.HasPrefix(h, ":") {
			h = "127.0.0.1" + h
		}
		slog.Info("веб-интерфейс", "откройте_в_браузере", "http://"+h)
	} else {
		slog.Info("подсказка", "msg", "веб не собран: см. README; корень http даёт подсказку; API: /api/v1/")
	}
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("http", "err", err)
		os.Exit(1)
	}
	// Дождаться фоновых задач до pool.Close (defer).
	shutdownWait := 15 * time.Second
	hook.Wait(shutdownWait)
	eng.Wait(shutdownWait)
	done := make(chan struct{})
	go func() {
		workers.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(shutdownWait):
		slog.Warn("shutdown: timeout waiting for workers", "timeout", shutdownWait.String())
	}
}
