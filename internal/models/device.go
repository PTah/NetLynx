package models

import "time"

type Device struct {
	ID                  int64      `json:"id"`
	Name                string     `json:"name"`
	Host                string     `json:"host"`
	Location            *string    `json:"location,omitempty"`
	UISPDeviceID        *string    `json:"uisp_device_id,omitempty"`
	UISPOverviewStatus  *string    `json:"uisp_overview_status,omitempty"` // из UISP overview.status (active/disconnected)
	SNMPVersion         string     `json:"snmp_version"`
	Community           *string    `json:"-"` // community не отдаём в API (см. HasCommunity)
	HasCommunity        bool       `json:"has_community"`
	V3User              *string    `json:"v3_user,omitempty"`
	V3AuthProtocol      *string    `json:"v3_auth_protocol,omitempty"`
	V3AuthPass          *string    `json:"-"` // не отдаём в JSON
	V3PrivProtocol      *string    `json:"v3_priv_protocol,omitempty"`
	V3PrivPass          *string    `json:"-"`
	V3EngineID          *string    `json:"v3_engine_id,omitempty"`
	PollIntervalSeconds      int        `json:"poll_interval_seconds"`
	UtilHighPct              *float32   `json:"util_high_pct,omitempty"`
	UtilOkPct                *float32   `json:"util_ok_pct,omitempty"`
	FDBPollIntervalSeconds   *int       `json:"fdb_poll_interval_seconds,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
	LastPollAt          *time.Time `json:"last_poll_at,omitempty"`
	LastSNMPOK          *bool      `json:"last_snmp_ok,omitempty"`
	LastSNMPError       *string    `json:"last_snmp_error,omitempty"`
	LastPingOK          *bool      `json:"last_ping_ok,omitempty"`
	LastPingAt          *time.Time `json:"last_ping_at,omitempty"`
	LastPingRTTMs       *int       `json:"last_ping_rtt_ms,omitempty"`
	/** nil = авто (ping/SNMP); true/false = ручная отметка онлайн/оффлайн */
	OnlineOverride      *bool      `json:"online_override"`
	/** Когда узел стал оффлайн; nil если сейчас онлайн или момент ещё не известен */
	OfflineSince        *time.Time `json:"offline_since,omitempty"`
	SysName             *string    `json:"sys_name,omitempty"`
	SysDescr            *string    `json:"sys_descr,omitempty"`
	ChassisMAC          *string    `json:"chassis_mac,omitempty"` // LLDP loc / bridge; для резолва MAC→inventory
	CPUProfile          *string    `json:"cpu_profile,omitempty"`
/** switch | router | ap | server | computer | phone | mfu | camera | other */
	DeviceCategory      string     `json:"device_category"`
	LastCPUPct            *float32   `json:"last_cpu_pct,omitempty"`
	LastCPUAt             *time.Time `json:"last_cpu_at,omitempty"`
	LastSysUptimeCs       *int64     `json:"last_sys_uptime_cs,omitempty"`
	FDBMonitoringStatus   string     `json:"fdb_monitoring_status,omitempty"`
	SSHUser               *string    `json:"ssh_user,omitempty"`
	SSHPassword           *string    `json:"-"`
	SSHPort               *int       `json:"ssh_port,omitempty"`
	SSHEnablePassword     *string    `json:"-"`
	SSHVendor             string     `json:"ssh_vendor,omitempty"`
	HasSSHPassword        bool       `json:"has_ssh_password,omitempty"`
	HasSSHEnablePassword  bool       `json:"has_ssh_enable_password,omitempty"`
	/** При link_trap_events_mode=per_device — писать LINK_* сразу из SNMP trap. */
	TrustLinkTraps        bool       `json:"trust_link_traps"`
}

type DeviceInterface struct {
	DeviceID         int64      `json:"device_id"`
	IfIndex          int        `json:"if_index"`
	IfDescr          *string    `json:"if_descr,omitempty"`
	IfName           *string    `json:"if_name,omitempty"`
	IfType           *int64     `json:"if_type,omitempty"`
	AdminStatus      *int       `json:"admin_status,omitempty"`
	OperStatus       *int       `json:"oper_status,omitempty"`
	IfSpeed          *int64     `json:"if_speed,omitempty"`
	IfHighSpeed      *int64     `json:"if_high_speed,omitempty"`
	PortRole         string     `json:"port_role"`
	/** switchport mode из show run (access/trunk); при наличии poller не перезаписывает port_role. */
	CLIPortMode      *string    `json:"cli_port_mode,omitempty"`
	CliAccessVlan    *int       `json:"cli_access_vlan,omitempty"`
	CliModeSyncedAt  *time.Time `json:"cli_mode_synced_at,omitempty"`
	HCInOctets       *int64     `json:"hc_in_octets,omitempty"`
	HCOutOctets      *int64     `json:"hc_out_octets,omitempty"`
	CountersPolledAt *time.Time `json:"counters_polled_at,omitempty"`
	UtilInPct        *float32   `json:"util_in_pct,omitempty"`
	UtilOutPct       *float32   `json:"util_out_pct,omitempty"`
	UtilMaxPct       *float32   `json:"util_max_pct,omitempty"`
	UtilHighActive   bool       `json:"util_high_active"`
	UtilHighPct      *float32   `json:"util_high_pct,omitempty"`
	UtilOkPct        *float32   `json:"util_ok_pct,omitempty"`
	EventIgnored     bool       `json:"event_ignored,omitempty"`
	/** off | soft | all — режим ignore list для порта. */
	IgnoreMode       string     `json:"ignore_mode,omitempty"`
	/** Подпись порта в NetLynx; если задана — показывается вместо SNMP ifDescr/ifAlias. */
	DescrOverride    *string    `json:"descr_override,omitempty"`
	/** description из show run; UI предпочитает её «железному» ifDescr, пока нет override. */
	CLIDescription   *string    `json:"cli_description,omitempty"`
	/** PoE: по SNMP, выдача питания; nil — неизвестно (MIB не ответил). */
	PoeActive        *bool      `json:"poe_active,omitempty"`
	/** PoE мощность на порту (ватты), если устройство отдаёт метрику. */
	PoePowerW        *float32   `json:"poe_power_w,omitempty"`
	/** Доминирующий VLAN по FDB на порту (для access; на trunk обычно не показывают). */
	VlanID           *int       `json:"vlan_id,omitempty"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type Event struct {
	ID        int64                  `json:"id"`
	DeviceID  int64                  `json:"device_id"`
	IfIndex   *int                   `json:"if_index,omitempty"`
	EventType string                 `json:"event_type"`
	Severity  string                 `json:"severity"`
	Payload   map[string]interface{} `json:"payload,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
}
