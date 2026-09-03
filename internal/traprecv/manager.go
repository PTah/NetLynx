package traprecv

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/live"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/notify"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

// Manager — hot-reload SNMP trap UDP listener по настройкам из БД.
type Manager struct {
	log       *slog.Logger
	st        *store.Store
	hub       *live.Hub
	hook      *notify.EventHook
	onAction  LinkActionFunc
	community string

	mu     sync.Mutex
	cancel context.CancelFunc
	addr   string // текущий listen, например ":9162"; пусто = выкл
}

func NewManager(
	log *slog.Logger,
	st *store.Store,
	hub *live.Hub,
	community string,
	hook *notify.EventHook,
	onAction LinkActionFunc,
) *Manager {
	if log == nil {
		log = slog.Default()
	}
	return &Manager{
		log:       log,
		st:        st,
		hub:       hub,
		hook:      hook,
		onAction:  onAction,
		community: community,
	}
}

// Status возвращает фактическое состояние сокета.
func (m *Manager) Status() (enabled bool, listenAddr string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.addr != "", m.addr
}

// Reload перечитывает snmp_trap_settings и перезапускает listener при необходимости.
func (m *Manager) Reload(parent context.Context) error {
	settings, err := m.st.GetSNMPTrapSettings(parent)
	if err != nil {
		return err
	}
	wantAddr := ""
	if settings.ListenEnabled {
		port := settings.ListenPort
		if port < 1 || port > 65535 {
			port = 9162
		}
		wantAddr = fmt.Sprintf(":%d", port)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if wantAddr == m.addr {
		return nil
	}
	m.stopLocked()
	if wantAddr == "" {
		m.log.Info("snmp trap receiver stopped")
		return nil
	}
	runCtx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.addr = wantAddr
	recv := New(m.log, m.st, m.hub, wantAddr, m.community, m.hook, m.onAction)
	go func(addr string) {
		if err := recv.Run(runCtx); err != nil && runCtx.Err() == nil {
			m.log.Error("snmp trap receiver", "listen", addr, "err", err)
		}
	}(wantAddr)
	m.log.Info("snmp trap receiver", "listen", wantAddr)
	return nil
}

// Stop останавливает listener (shutdown процесса).
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopLocked()
}

func (m *Manager) stopLocked() {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
		// Дать Listen выйти.
		time.Sleep(200 * time.Millisecond)
	}
	m.addr = ""
}
