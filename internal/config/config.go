package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config загружается из переменных окружения (см. .env.example).
type Config struct {
	DatabaseURL       string
	HTTPAddr          string
	// PollSchedulerSeconds — как часто планировщик проверяет, какие узлы пора опросить (не путать с poll_interval_seconds устройства в БД).
	PollSchedulerSeconds int
	// AccessPortLongIdle — порог «порт был пуст N времени», затем появился MAC (событие ACCESS_PORT_LONG_IDLE_DEVICE).
	AccessPortLongIdle time.Duration
	FDBPollInterval   time.Duration
	FDBLearnDuration  time.Duration
	// Авто-классификация trunk по FDB: если на порту >= MinMACs и MAC из >= MinVLANs, порт считаем trunk для FDB-событий.
	FDBAutoTrunkMinMACs  int
	FDBAutoTrunkMinVLANs int
	// Фолбэк, когда VLAN из Q-BRIDGE-MIB недоступны: trunk по одному MACCount (порог обычно выше).
	FDBAutoTrunkFallbackMinMACs int
	PortUtilHighPct   float64
	PortUtilOKPct     float64
	AccessTokenTTL    time.Duration
	RefreshTokenTTL   time.Duration
	// WebStaticDir — абсолютный путь к каталогу с собранным фронтом (index.html). Пусто = не отдавать веб с Go.
	WebStaticDir string
	// AuthDisabled — явный отказ от auth (только lab). Иначе нужен NETLYNX_ADMIN_PASSWORD.
	AuthDisabled  bool
	AdminUser     string
	AdminPassword string
	JWTSecret     string
	// CookieSecure — Secure-флаг для refresh-cookie (за TLS/HTTPS proxy).
	CookieSecure bool
	// TrustProxy — доверять X-Real-IP / X-Forwarded-For для client IP (login rate-limit).
	// По умолчанию false: только RemoteAddr (защита от XFF-спуфинга).
	TrustProxy bool

	// SSH PoE fallback (опционально): используется только когда SNMP не дал PoE по порту.
	SSHPOEFallbackEnabled bool
	SSHPOEUser            string
	SSHPOEPass            string
	SSHPOEEnablePass      string
	SSHPOEPort            int
	SSHPOETimeout         time.Duration
	SSHPOEKnownHosts      string // путь к known_hosts (обязателен при включённом SSH PoE)

	// SNMP trap receiver (опционально): если адрес задан, сервер принимает traps на UDP.
	SNMPTrapListenAddr string
	// Если задано, принимаются только traps с этим community (для v1/v2c).
	SNMPTrapCommunity string

	MetricsEnabled       bool
	MetricsRetentionDays int

	// MAC investigation / flapping.
	// SyslogListenAddr — UDP syslog (пусто = выкл). Пример: ":9514".
	SyslogListenAddr      string
	MACFlapMinMoves       int           // ≥K смен порта за окно → MAC_FLAPPING
	MACFlapWindow         time.Duration // окно подсчёта moves
	MACFlapDebounce       time.Duration // не повторять событие чаще
	MACMovesRetentionDays int           // prune mac_fdb_moves

	// Broadcast storm heuristic (poller): ≥N ports above util % + FDB growth.
	BroadcastStormMinPorts        int
	BroadcastStormUtilPct         float64
	BroadcastStormFDBMinGrowth    int
	BroadcastStormFDBMinGrowthPct float64
	BroadcastStormDebounce        time.Duration

	// FDB daily snapshots для истории «где MAC был N дней назад».
	FDBSnapshotEnabled       bool
	FDBSnapshotInterval      time.Duration
	FDBSnapshotRetentionDays int
	// FDBStaleClearDays — авто-очистка live FDB (device_fdb_entries), если last_fdb_poll_at старше N дней (0 = выкл).
	FDBStaleClearDays int

	// История running-config (SSH snapshots + diff).
	ConfigSnapshotEnabled       bool
	ConfigSnapshotInterval      time.Duration
	ConfigSnapshotRetentionDays int

	// PublicBaseURL — публичный URL UI для ссылок в письмах (http://host:8080).
	PublicBaseURL string
}

func Load() (Config, error) {
	c := Config{
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		HTTPAddr:        getenv("HTTP_ADDR", ":8080"),
		PortUtilHighPct: 90,
		PortUtilOKPct:   85,
	}
	if c.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if v := os.Getenv("POLL_SCHEDULER_SECONDS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 5 || n > 600 {
			return Config{}, fmt.Errorf("POLL_SCHEDULER_SECONDS: invalid value %q (ожидается 5–600)", v)
		}
		c.PollSchedulerSeconds = n
	} else {
		c.PollSchedulerSeconds = 10
	}
	if v := os.Getenv("ACCESS_PORT_LONG_IDLE_HOURS"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f < 1 || f > 8760 {
			return Config{}, fmt.Errorf("ACCESS_PORT_LONG_IDLE_HOURS: invalid value %q", v)
		}
		c.AccessPortLongIdle = time.Duration(f * float64(time.Hour))
	} else {
		c.AccessPortLongIdle = 72 * time.Hour
	}
	if v := os.Getenv("FDB_POLL_INTERVAL_SECONDS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 30 {
			return Config{}, fmt.Errorf("FDB_POLL_INTERVAL_SECONDS: invalid value %q", v)
		}
		c.FDBPollInterval = time.Duration(n) * time.Second
	} else {
		c.FDBPollInterval = 15 * time.Minute
	}
	if v := os.Getenv("FDB_LEARN_SECONDS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return Config{}, fmt.Errorf("FDB_LEARN_SECONDS: invalid value %q", v)
		}
		c.FDBLearnDuration = time.Duration(n) * time.Second
	} else {
		c.FDBLearnDuration = 30 * time.Minute
	}
	if v := os.Getenv("FDB_AUTO_TRUNK_MIN_MACS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return Config{}, fmt.Errorf("FDB_AUTO_TRUNK_MIN_MACS: invalid value %q", v)
		}
		c.FDBAutoTrunkMinMACs = n
	} else {
		c.FDBAutoTrunkMinMACs = 8
	}
	if v := os.Getenv("FDB_AUTO_TRUNK_MIN_VLANS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 2 {
			return Config{}, fmt.Errorf("FDB_AUTO_TRUNK_MIN_VLANS: invalid value %q", v)
		}
		c.FDBAutoTrunkMinVLANs = n
	} else {
		c.FDBAutoTrunkMinVLANs = 2
	}
	if v := os.Getenv("FDB_AUTO_TRUNK_FALLBACK_MIN_MACS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return Config{}, fmt.Errorf("FDB_AUTO_TRUNK_FALLBACK_MIN_MACS: invalid value %q", v)
		}
		c.FDBAutoTrunkFallbackMinMACs = n
	} else {
		c.FDBAutoTrunkFallbackMinMACs = 12
	}
	if v := os.Getenv("PORT_UTIL_HIGH_PCT"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return Config{}, err
		}
		c.PortUtilHighPct = f
	}
	if v := os.Getenv("PORT_UTIL_OK_PCT"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return Config{}, err
		}
		c.PortUtilOKPct = f
	}

	c.AdminUser = envLegacy("NETLYNX_ADMIN_USER", "INETOR_ADMIN_USER", "admin")
	c.AdminPassword = strings.TrimSpace(envLegacy("NETLYNX_ADMIN_PASSWORD", "INETOR_ADMIN_PASSWORD", ""))
	c.AuthDisabled = strings.EqualFold(strings.TrimSpace(envLegacy("NETLYNX_AUTH_DISABLED", "INETOR_AUTH_DISABLED", "")), "true")
	c.TrustProxy = strings.EqualFold(strings.TrimSpace(envLegacy("NETLYNX_TRUST_PROXY", "INETOR_TRUST_PROXY", "")), "true")
	if c.AuthDisabled && c.AdminPassword != "" {
		return Config{}, fmt.Errorf("NETLYNX_AUTH_DISABLED=true несовместим с заданным NETLYNX_ADMIN_PASSWORD")
	}
	if !c.AuthDisabled && c.AdminPassword == "" {
		return Config{}, fmt.Errorf("NETLYNX_ADMIN_PASSWORD обязателен (или NETLYNX_AUTH_DISABLED=true для lab)")
	}
	// JWT: NETLYNX_JWT_SECRET — опциональный seed; постоянный секрет в app_secrets (EnsureJWTSecret после Migrate).
	if jwt := strings.TrimSpace(envLegacy("NETLYNX_JWT_SECRET", "INETOR_JWT_SECRET", "")); jwt != "" {
		if len(jwt) < 32 {
			return Config{}, fmt.Errorf("NETLYNX_JWT_SECRET должен быть не короче 32 символов")
		}
		c.JWTSecret = jwt
	}
	// CookieSecure: по умолчанию true; для plain HTTP LAN — NETLYNX_COOKIE_SECURE=false.
	if v, ok := envLegacyLookup("NETLYNX_COOKIE_SECURE", "INETOR_COOKIE_SECURE"); ok {
		c.CookieSecure = strings.EqualFold(strings.TrimSpace(v), "true")
	} else {
		c.CookieSecure = true
	}

	c.SSHPOEFallbackEnabled = strings.EqualFold(strings.TrimSpace(getenv("SSH_POE_FALLBACK_ENABLED", "false")), "true")
	c.SSHPOEUser = strings.TrimSpace(os.Getenv("SSH_POE_USER"))
	c.SSHPOEPass = os.Getenv("SSH_POE_PASS")
	c.SSHPOEEnablePass = os.Getenv("SSH_POE_ENABLE_PASS")
	c.SSHPOEPort = 22
	if v := strings.TrimSpace(os.Getenv("SSH_POE_PORT")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 || n > 65535 {
			return Config{}, fmt.Errorf("SSH_POE_PORT: invalid value %q", v)
		}
		c.SSHPOEPort = n
	}
	c.SSHPOETimeout = 5 * time.Second
	if v := strings.TrimSpace(os.Getenv("SSH_POE_TIMEOUT_SECONDS")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 60 {
			return Config{}, fmt.Errorf("SSH_POE_TIMEOUT_SECONDS: invalid value %q", v)
		}
		c.SSHPOETimeout = time.Duration(n) * time.Second
	}
	c.SSHPOEKnownHosts = strings.TrimSpace(os.Getenv("SSH_POE_KNOWN_HOSTS"))
	if c.SSHPOEKnownHosts == "" {
		c.SSHPOEKnownHosts = filepath.ToSlash("/var/lib/netlynx/ssh_known_hosts")
	}
	// При SSH_POE_FALLBACK_ENABLED без known_hosts dial вернёт ошибку (не валим весь процесс).

	c.SNMPTrapListenAddr = strings.TrimSpace(os.Getenv("SNMP_TRAP_LISTEN_ADDR"))
	if c.SNMPTrapListenAddr == "" {
		// По умолчанию слушаем traps на непривилегированном порту; off/disabled — выключить.
		c.SNMPTrapListenAddr = ":9162"
	}
	if strings.EqualFold(c.SNMPTrapListenAddr, "off") || strings.EqualFold(c.SNMPTrapListenAddr, "disabled") {
		c.SNMPTrapListenAddr = ""
	}
	c.SNMPTrapCommunity = strings.TrimSpace(os.Getenv("SNMP_TRAP_COMMUNITY"))

	c.MetricsEnabled = !strings.EqualFold(strings.TrimSpace(getenv("METRICS_ENABLED", "true")), "false")
	c.MetricsRetentionDays = 7
	if v := os.Getenv("METRICS_RETENTION_DAYS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 365 {
			return Config{}, fmt.Errorf("METRICS_RETENTION_DAYS: invalid value %q", v)
		}
		c.MetricsRetentionDays = n
	}

	c.SyslogListenAddr = strings.TrimSpace(os.Getenv("NETLYNX_SYSLOG_LISTEN"))
	if strings.EqualFold(c.SyslogListenAddr, "off") || strings.EqualFold(c.SyslogListenAddr, "disabled") {
		c.SyslogListenAddr = ""
	}
	c.MACFlapMinMoves = 3
	if v := os.Getenv("MAC_FLAP_MIN_MOVES"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 2 || n > 100 {
			return Config{}, fmt.Errorf("MAC_FLAP_MIN_MOVES: invalid value %q (ожидается 2–100)", v)
		}
		c.MACFlapMinMoves = n
	}
	c.MACFlapWindow = time.Hour
	if v := os.Getenv("MAC_FLAP_WINDOW_SECONDS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 60 || n > 86400 {
			return Config{}, fmt.Errorf("MAC_FLAP_WINDOW_SECONDS: invalid value %q", v)
		}
		c.MACFlapWindow = time.Duration(n) * time.Second
	}
	c.MACFlapDebounce = 15 * time.Minute
	if v := os.Getenv("MAC_FLAP_DEBOUNCE_SECONDS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 60 || n > 86400 {
			return Config{}, fmt.Errorf("MAC_FLAP_DEBOUNCE_SECONDS: invalid value %q", v)
		}
		c.MACFlapDebounce = time.Duration(n) * time.Second
	}
	c.MACMovesRetentionDays = 14
	if v := os.Getenv("MAC_MOVES_RETENTION_DAYS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 365 {
			return Config{}, fmt.Errorf("MAC_MOVES_RETENTION_DAYS: invalid value %q", v)
		}
		c.MACMovesRetentionDays = n
	}

	c.BroadcastStormMinPorts = 3
	if v := os.Getenv("BROADCAST_STORM_MIN_PORTS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 2 || n > 48 {
			return Config{}, fmt.Errorf("BROADCAST_STORM_MIN_PORTS: invalid value %q (2–48)", v)
		}
		c.BroadcastStormMinPorts = n
	}
	c.BroadcastStormUtilPct = 80
	if v := os.Getenv("BROADCAST_STORM_UTIL_PCT"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f < 50 || f > 100 {
			return Config{}, fmt.Errorf("BROADCAST_STORM_UTIL_PCT: invalid value %q (50–100)", v)
		}
		c.BroadcastStormUtilPct = f
	}
	c.BroadcastStormFDBMinGrowth = 5
	if v := os.Getenv("BROADCAST_STORM_FDB_MIN_GROWTH"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 10000 {
			return Config{}, fmt.Errorf("BROADCAST_STORM_FDB_MIN_GROWTH: invalid value %q", v)
		}
		c.BroadcastStormFDBMinGrowth = n
	}
	c.BroadcastStormFDBMinGrowthPct = 2
	if v := os.Getenv("BROADCAST_STORM_FDB_MIN_GROWTH_PCT"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f < 0.1 || f > 100 {
			return Config{}, fmt.Errorf("BROADCAST_STORM_FDB_MIN_GROWTH_PCT: invalid value %q", v)
		}
		c.BroadcastStormFDBMinGrowthPct = f
	}
	c.BroadcastStormDebounce = 30 * time.Minute
	if v := os.Getenv("BROADCAST_STORM_DEBOUNCE_SECONDS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 60 || n > 86400 {
			return Config{}, fmt.Errorf("BROADCAST_STORM_DEBOUNCE_SECONDS: invalid value %q", v)
		}
		c.BroadcastStormDebounce = time.Duration(n) * time.Second
	}

	c.FDBSnapshotEnabled = !strings.EqualFold(strings.TrimSpace(getenv("FDB_SNAPSHOT_ENABLED", "true")), "false")
	c.FDBSnapshotInterval = 24 * time.Hour
	if v := os.Getenv("FDB_SNAPSHOT_INTERVAL_HOURS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 12 || n > 168 {
			return Config{}, fmt.Errorf("FDB_SNAPSHOT_INTERVAL_HOURS: invalid value %q (12–168)", v)
		}
		c.FDBSnapshotInterval = time.Duration(n) * time.Hour
	}
	c.FDBSnapshotRetentionDays = 30
	if v := os.Getenv("FDB_SNAPSHOT_RETENTION_DAYS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 3 || n > 365 {
			return Config{}, fmt.Errorf("FDB_SNAPSHOT_RETENTION_DAYS: invalid value %q (3–365)", v)
		}
		c.FDBSnapshotRetentionDays = n
	}
	c.FDBStaleClearDays = 60
	if v := os.Getenv("FDB_STALE_CLEAR_DAYS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > 3650 {
			return Config{}, fmt.Errorf("FDB_STALE_CLEAR_DAYS: invalid value %q (0–3650, 0=выкл)", v)
		}
		c.FDBStaleClearDays = n
	}

	c.ConfigSnapshotEnabled = !strings.EqualFold(strings.TrimSpace(getenv("CONFIG_SNAPSHOT_ENABLED", "true")), "false")
	c.ConfigSnapshotInterval = 24 * time.Hour
	if v := os.Getenv("CONFIG_SNAPSHOT_INTERVAL_HOURS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 168 {
			return Config{}, fmt.Errorf("CONFIG_SNAPSHOT_INTERVAL_HOURS: invalid value %q (1–168)", v)
		}
		c.ConfigSnapshotInterval = time.Duration(n) * time.Hour
	}
	c.ConfigSnapshotRetentionDays = 90
	if v := os.Getenv("CONFIG_SNAPSHOT_RETENTION_DAYS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 7 || n > 730 {
			return Config{}, fmt.Errorf("CONFIG_SNAPSHOT_RETENTION_DAYS: invalid value %q (7–730)", v)
		}
		c.ConfigSnapshotRetentionDays = n
	}

	c.AccessTokenTTL = 15 * time.Minute
	if v := envLegacy("NETLYNX_ACCESS_TTL_SECONDS", "INETOR_ACCESS_TTL_SECONDS", ""); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 60 {
			return Config{}, fmt.Errorf("NETLYNX_ACCESS_TTL_SECONDS: invalid value %q", v)
		}
		c.AccessTokenTTL = time.Duration(n) * time.Second
	}
	c.RefreshTokenTTL = 7 * 24 * time.Hour
	if v := envLegacy("NETLYNX_REFRESH_TTL_SECONDS", "INETOR_REFRESH_TTL_SECONDS", ""); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 300 {
			return Config{}, fmt.Errorf("NETLYNX_REFRESH_TTL_SECONDS: invalid value %q", v)
		}
		c.RefreshTokenTTL = time.Duration(n) * time.Second
	}

	webDist := strings.TrimSpace(os.Getenv("WEB_DIST"))
	if webDist == "" {
		webDist = "web/dist"
	}
	if webDist != "-" {
		idx := filepath.Join(webDist, "index.html")
		if fi, err := os.Stat(idx); err == nil && !fi.IsDir() {
			abs, err := filepath.Abs(webDist)
			if err != nil {
				return Config{}, fmt.Errorf("WEB_DIST: %w", err)
			}
			c.WebStaticDir = abs
		}
	}

	c.PublicBaseURL = strings.TrimRight(strings.TrimSpace(envLegacy("NETLYNX_PUBLIC_URL", "INETOR_PUBLIC_URL", "")), "/")

	return c, nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// envLegacy читает newKey, иначе legacyKey (миграция Invetor → NetLynx).
func envLegacy(newKey, legacyKey, def string) string {
	if v := strings.TrimSpace(os.Getenv(newKey)); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv(legacyKey)); v != "" {
		return v
	}
	return def
}

func envLegacyLookup(newKey, legacyKey string) (string, bool) {
	if v, ok := os.LookupEnv(newKey); ok {
		return v, true
	}
	if v, ok := os.LookupEnv(legacyKey); ok {
		return v, true
	}
	return "", false
}
