export type Device = {
  id: number;
  name: string;
  host: string;
  location?: string | null;
  /** switch | router | ap | server | computer | phone | mfu | camera | other */
  device_category?: string;
  uisp_device_id?: string | null;
  /** Состояние связи с UISP (overview.status): active, disconnected, … */
  uisp_overview_status?: string | null;
  snmp_version: string;
  community?: string | null;
  has_community?: boolean;
  ssh_user?: string | null;
  ssh_port?: number | null;
  ssh_vendor?: string | null;
  has_ssh_password?: boolean;
  has_ssh_enable_password?: boolean;
  v3_user?: string | null;
  v3_auth_protocol?: string | null;
  v3_priv_protocol?: string | null;
  v3_engine_id?: string | null;
  poll_interval_seconds: number;
  last_snmp_ok?: boolean | null;
  last_snmp_error?: string | null;
  /** ICMP ping с сервера NetLynx */
  last_ping_ok?: boolean | null;
  last_ping_at?: string | null;
  last_ping_rtt_ms?: number | null;
  /** null = авто; true/false = ручная отметка онлайн/оффлайн (legacy) */
  online_override?: boolean | null;
  /** auto | ping | online | offline */
  reachability_mode?: string | null;
  /** При mode=per_device — мгновенные LINK_* из SNMP trap */
  trust_link_traps?: boolean;
  /** ISO: когда узел стал оффлайн (если сейчас оффлайн) */
  offline_since?: string | null;
  sys_name?: string | null;
  /** Chassis MAC (LLDP loc / bridge), для резолва соседей */
  chassis_mac?: string | null;
  last_poll_at?: string | null;
  cpu_profile?: string | null;
  last_cpu_pct?: number | null;
  last_cpu_at?: string | null;
  /** sysUpTime (SNMP TimeTicks, сотые доли секунды) при last_poll_at */
  last_sys_uptime_cs?: number | null;
  /** Printer-MIB prtMarkerLifeCount */
  last_page_count?: number | null;
  last_toners?: PrinterToner[] | null;
  /** Снимок ИБП (UPS-MIB, RFC 1628) */
  last_ups?: UPSSnapshot | null;
  last_ups_at?: string | null;
  fdb_monitoring_status?: string | null;
  util_high_pct?: number | null;
  util_ok_pct?: number | null;
  fdb_poll_interval_seconds?: number | null;
  /** Тяжёлый опрос: LLDP/CDP/ARP/PoE/STP/FDB и роли портов; null = POLL_HEAVY_SECONDS */
  heavy_poll_interval_seconds?: number | null;
  last_heavy_poll_at?: string | null;
};

export type PrinterToner = {
  key: string;
  label: string;
  description?: string;
  level?: number | null;
  max?: number | null;
  pct?: number | null;
  unit?: number;
  unknown?: boolean;
  remaining_ok?: boolean;
  metric_type?: string;
};

/** Снимок UPS-MIB (RFC 1628) — см. docs/UPS.md. */
export type UPSSnapshot = {
  manufacturer?: string;
  model?: string;
  ident_name?: string;
  agent_software?: string;
  /** 1 unknown, 2 normal, 3 low, 4 depleted */
  battery_status?: number;
  seconds_on_battery?: number | null;
  runtime_min?: number | null;
  battery_charge_pct?: number | null;
  battery_voltage_v?: number | null;
  battery_temp_c?: number | null;
  on_battery?: boolean;
  input_voltage_v?: number | null;
  input_frequency_hz?: number | null;
  /** 1 other, 2 none, 3 normal, 4 bypass, 5 battery, 6 booster, 7 reducer */
  output_source?: number;
  output_voltage_v?: number | null;
  output_current_a?: number | null;
  output_power_w?: number | null;
  output_load_pct?: number | null;
  alarms_present?: number | null;
  config_input_voltage_v?: number | null;
  low_battery_runtime_min?: number | null;
  polled_at?: string;
};

export type EventRow = {
  id: number;
  device_id: number;
  if_index?: number | null;
  event_type: string;
  severity: string;
  payload?: Record<string, unknown>;
  created_at: string;
};
